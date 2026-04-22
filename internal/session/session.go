package session

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/routerarchitects/nats-agent-core/internal/runtimeerr"
)

// Manager owns the runtime NATS, JetStream, KV, and health session state.
type Manager struct {
	mu        sync.RWMutex
	cfg       Config
	effective EffectiveConfig
	hooks     Hooks

	nc *nats.Conn
	js jetstream.JetStream
	kv jetstream.KeyValue

	health   HealthSnapshot
	starting bool
	closing  bool
}

// NewManager constructs a session manager with normalized runtime defaults.
func NewManager(cfg Config, hooks Hooks) (*Manager, error) {
	effective, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Manager{
		cfg:       cfg,
		effective: effective,
		hooks:     hooks,
		health: HealthSnapshot{
			State: StateNew,
		},
	}, nil
}

// EffectiveConfig returns the normalized config currently used by the runtime.
func (m *Manager) EffectiveConfig() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.effective.Config
}

// HealthSnapshot returns the latest read-only transport health snapshot.
func (m *Manager) HealthSnapshot() HealthSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health
}

// DesiredConfigBucket returns the configured desired-config KV bucket.
func (m *Manager) DesiredConfigBucket() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.effective.Config.KV.Bucket
}

// DesiredConfigKeyPattern returns the configured desired-config KV key pattern.
func (m *Manager) DesiredConfigKeyPattern() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.effective.Config.KV.KeyPattern
}

// KVTimeout returns the configured KV timeout used for storage operations.
func (m *Manager) KVTimeout() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.effective.Config.Timeouts.KVTimeout
}

// Start initializes the runtime connection, JetStream handle, and KV bucket.
func (m *Manager) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeConnectionFailed,
			Op:        "start",
			Message:   "start context is not usable",
			Retryable: true,
			Err:       err,
		}
	}

	m.mu.Lock()
	if m.starting {
		m.mu.Unlock()
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeValidation,
			Op:        "start",
			Message:   "start already in progress",
			Retryable: false,
		}
	}
	if m.nc != nil && m.nc.Status() != nats.CLOSED {
		m.mu.Unlock()
		return nil
	}
	m.starting = true
	m.setStateLocked(StateConnecting)
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.starting = false
		m.mu.Unlock()
	}()

	nc, err := m.connectNATS()
	if err != nil {
		m.mu.Lock()
		m.setDegradedLocked(err)
		m.mu.Unlock()
		if m.hooks.Metrics != nil {
			m.hooks.Metrics.IncConnect("failure")
		}
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeConnectionFailed,
			Op:        "start_connect",
			Message:   "failed to connect to NATS",
			Retryable: true,
			Err:       err,
		}
	}

	js, err := m.newJetStream(nc)
	if err != nil {
		nc.Close()
		m.mu.Lock()
		m.setDegradedLocked(err)
		m.mu.Unlock()
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeJetStreamFailed,
			Op:        "start_jetstream",
			Message:   "failed to initialize JetStream",
			Retryable: true,
			Err:       err,
		}
	}

	setupCtx, cancel := m.withKVTimeout(ctx)
	defer cancel()

	kv, err := m.bindOrCreateKV(setupCtx, js)
	if err != nil {
		nc.Close()
		m.mu.Lock()
		m.setDegradedLocked(err)
		m.mu.Unlock()
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeJetStreamFailed,
			Op:        "start_kv",
			Message:   "failed to bind or create desired-config KV bucket",
			Retryable: true,
			Err:       err,
		}
	}

	m.mu.Lock()
	m.nc = nc
	m.js = js
	m.kv = kv
	m.setConnectedLocked(nc.ConnectedUrl(), true, true)
	m.mu.Unlock()

	if m.hooks.Metrics != nil {
		m.hooks.Metrics.IncConnect("success")
	}

	return nil
}

// Close drains and tears down the runtime session.
func (m *Manager) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return nil
	}
	if m.nc == nil {
		m.setClosedLocked(nil)
		m.mu.Unlock()
		return nil
	}

	m.closing = true
	m.setStateLocked(StateDraining)

	nc := m.nc
	shutdownTimeout := m.effective.Config.Timeouts.ShutdownTimeout
	m.mu.Unlock()

	drainErr := drainConnection(ctx, nc, shutdownTimeout)
	if drainErr != nil {
		nc.Close()
	}

	m.mu.Lock()
	m.nc = nil
	m.js = nil
	m.kv = nil
	m.closing = false
	if drainErr != nil {
		m.setClosedLocked(drainErr)
	} else {
		m.setClosedLocked(nil)
	}
	m.mu.Unlock()

	if drainErr != nil {
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeShutdown,
			Op:        "close",
			Message:   "failed to drain NATS connection cleanly",
			Retryable: true,
			Err:       drainErr,
		}
	}

	return nil
}

// KeyValue returns the active KV handle when runtime is connected.
func (m *Manager) KeyValue() (jetstream.KeyValue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.nc == nil || m.nc.Status() != nats.CONNECTED || m.kv == nil {
		return nil, &runtimeerr.Error{
			Code:      runtimeerr.CodeDisconnected,
			Op:        "key_value",
			Message:   "client runtime is not connected",
			Retryable: true,
		}
	}

	return m.kv, nil
}

func (m *Manager) connectNATS() (*nats.Conn, error) {
	opts, err := m.buildNATSOptions()
	if err != nil {
		return nil, err
	}

	servers := strings.Join(m.effective.Config.NATS.Servers, ",")
	return nats.Connect(servers, opts...)
}

func (m *Manager) buildNATSOptions() ([]nats.Option, error) {
	ncfg := m.effective.Config.NATS
	opts := []nats.Option{
		nats.Timeout(ncfg.ConnectTimeout),
		nats.RetryOnFailedConnect(ncfg.RetryOnFailedConnect),
		nats.MaxReconnects(ncfg.MaxReconnects),
		nats.ReconnectWait(ncfg.ReconnectWait),
		nats.ReconnectBufSize(ncfg.ReconnectBufSize),
		nats.DisconnectErrHandler(m.onDisconnect),
		nats.ReconnectHandler(m.onReconnect),
		nats.ClosedHandler(m.onClosed),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			m.onAsyncError(err)
		}),
	}

	clientName := strings.TrimSpace(ncfg.ClientName)
	if clientName == "" {
		clientName = strings.TrimSpace(m.effective.Config.AgentName)
	}
	if clientName != "" {
		opts = append(opts, nats.Name(clientName))
	}

	if strings.TrimSpace(ncfg.CredentialsFile) != "" {
		opts = append(opts, nats.UserCredentials(ncfg.CredentialsFile))
	} else if strings.TrimSpace(ncfg.UserJWTFile) != "" && strings.TrimSpace(ncfg.NKeySeedFile) != "" {
		opts = append(opts, nats.UserCredentials(ncfg.UserJWTFile, ncfg.NKeySeedFile))
	} else if strings.TrimSpace(ncfg.NKeySeedFile) != "" {
		nkeyOpt, err := nats.NkeyOptionFromSeed(ncfg.NKeySeedFile)
		if err != nil {
			return nil, err
		}
		opts = append(opts, nkeyOpt)
	} else if strings.TrimSpace(ncfg.Username) != "" || strings.TrimSpace(ncfg.Password) != "" {
		opts = append(opts, nats.UserInfo(ncfg.Username, ncfg.Password))
	} else if strings.TrimSpace(ncfg.Token) != "" {
		opts = append(opts, nats.Token(ncfg.Token))
	}

	tlsCfg, err := buildTLSConfig(ncfg.TLS)
	if err != nil {
		return nil, err
	}
	if tlsCfg != nil {
		opts = append(opts, nats.Secure(tlsCfg))
	}

	return opts, nil
}

func buildTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		ServerName:         cfg.ServerName,
	}

	if strings.TrimSpace(cfg.CAFile) != "" {
		caPEM, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read tls ca file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("tls ca file contains no valid certificates")
		}
		tlsCfg.RootCAs = pool
	}

	certFile := strings.TrimSpace(cfg.CertFile)
	keyFile := strings.TrimSpace(cfg.KeyFile)
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return nil, errors.New("both tls cert_file and key_file are required when configuring client certificates")
		}
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load tls client cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

func (m *Manager) newJetStream(nc *nats.Conn) (jetstream.JetStream, error) {
	opts := []jetstream.JetStreamOpt{
		jetstream.WithDefaultTimeout(m.effective.Config.JetStream.DefaultTimeout),
	}
	if domain := strings.TrimSpace(m.effective.Config.JetStream.Domain); domain != "" {
		return jetstream.NewWithDomain(nc, domain, opts...)
	}
	if prefix := strings.TrimSpace(m.effective.Config.JetStream.APIPrefix); prefix != "" {
		return jetstream.NewWithAPIPrefix(nc, prefix, opts...)
	}
	return jetstream.New(nc, opts...)
}

func (m *Manager) bindOrCreateKV(ctx context.Context, js jetstream.JetStream) (jetstream.KeyValue, error) {
	bucket := m.effective.Config.KV.Bucket

	kv, err := js.KeyValue(ctx, bucket)
	if err == nil {
		return kv, nil
	}
	if !isBucketNotFound(err) {
		return nil, err
	}
	if !m.effective.Config.KV.AutoCreateBucket {
		return nil, err
	}

	kvCfg := jetstream.KeyValueConfig{
		Bucket:       bucket,
		History:      m.effective.Config.KV.History,
		TTL:          m.effective.Config.KV.TTL,
		MaxValueSize: m.effective.Config.KV.MaxValueSize,
		Replicas:     m.effective.Config.KV.Replicas,
	}

	switch strings.ToLower(m.effective.Config.KV.Storage) {
	case "memory":
		kvCfg.Storage = jetstream.MemoryStorage
	default:
		kvCfg.Storage = jetstream.FileStorage
	}

	return js.CreateKeyValue(ctx, kvCfg)
}

func (m *Manager) withKVTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, m.effective.Config.Timeouts.KVTimeout)
}

func drainConnection(ctx context.Context, nc *nats.Conn, timeout time.Duration) error {
	if nc == nil {
		return nil
	}

	drainCtx := ctx
	cancel := func() {}
	if _, ok := ctx.Deadline(); !ok && timeout > 0 {
		drainCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- nc.Drain()
	}()

	select {
	case err := <-done:
		return err
	case <-drainCtx.Done():
		return drainCtx.Err()
	}
}
