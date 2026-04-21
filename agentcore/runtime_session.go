package agentcore

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	defaultConnectTimeout   = 5 * time.Second
	defaultReconnectWait    = 2 * time.Second
	defaultMaxReconnects    = -1
	defaultJetStreamTimeout = 5 * time.Second

	defaultPublishTimeout   = 5 * time.Second
	defaultSubscribeTimeout = 5 * time.Second
	defaultKVTimeout        = 5 * time.Second
	defaultShutdownTimeout  = 10 * time.Second
	defaultHandlerWarnAfter = 5 * time.Second

	defaultPublishAttempts = 3
	defaultPublishBackoff  = 200 * time.Millisecond

	defaultKVBucket   = "cfg_desired"
	defaultKVKey      = "desired.%s"
	defaultKVHistory  = uint8(1)
	defaultKVReplicas = 1
	defaultKVStorage  = "file"
)

type runtimeHooks struct {
	Logger    Logger
	Metrics   Metrics
	ErrorSink func(error)
}

type runtimeSession struct {
	mu        sync.RWMutex
	cfg       Config
	effective Config
	hooks     runtimeHooks

	nc *nats.Conn
	js jetstream.JetStream
	kv jetstream.KeyValue

	health   HealthSnapshot
	starting bool
	closing  bool
}

func newRuntimeSession(cfg Config, hooks runtimeHooks) (*runtimeSession, error) {
	effective, err := normalizeRuntimeConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &runtimeSession{
		cfg:       cfg,
		effective: effective,
		hooks:     hooks,
		health: HealthSnapshot{
			State: StateNew,
		},
	}, nil
}

func (s *runtimeSession) effectiveConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.effective
}

func (s *runtimeSession) healthSnapshot() HealthSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.health
}

func (s *runtimeSession) start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return &Error{
			Code:      CodeConnectionFailed,
			Op:        "start",
			Message:   "start context is not usable",
			Retryable: true,
			Err:       err,
		}
	}

	s.mu.Lock()
	if s.starting {
		s.mu.Unlock()
		return &Error{Code: CodeValidation, Op: "start", Message: "start already in progress", Retryable: false}
	}
	if s.nc != nil && s.nc.Status() != nats.CLOSED {
		s.mu.Unlock()
		return nil
	}
	s.starting = true
	s.setStateLocked(StateConnecting)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.starting = false
		s.mu.Unlock()
	}()

	nc, err := s.connectNATS()
	if err != nil {
		s.mu.Lock()
		s.setDegradedLocked(err)
		s.mu.Unlock()
		if s.hooks.Metrics != nil {
			s.hooks.Metrics.IncConnect("failure")
		}
		return &Error{Code: CodeConnectionFailed, Op: "start_connect", Message: "failed to connect to NATS", Retryable: true, Err: err}
	}

	js, err := s.newJetStream(nc)
	if err != nil {
		nc.Close()
		s.mu.Lock()
		s.setDegradedLocked(err)
		s.mu.Unlock()
		return &Error{Code: CodeJetStreamFailed, Op: "start_jetstream", Message: "failed to initialize JetStream", Retryable: true, Err: err}
	}

	setupCtx, cancel := withTimeout(ctx, s.effective.Timeouts.KVTimeout)
	defer cancel()

	kv, err := s.bindOrCreateKV(setupCtx, js)
	if err != nil {
		nc.Close()
		s.mu.Lock()
		s.setDegradedLocked(err)
		s.mu.Unlock()
		return &Error{Code: CodeJetStreamFailed, Op: "start_kv", Message: "failed to bind or create desired-config KV bucket", Retryable: true, Err: err}
	}

	s.mu.Lock()
	s.nc = nc
	s.js = js
	s.kv = kv
	s.setConnectedLocked(nc.ConnectedUrl(), true, true)
	s.mu.Unlock()

	if s.hooks.Metrics != nil {
		s.hooks.Metrics.IncConnect("success")
	}

	return nil
}

func (s *runtimeSession) close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return nil
	}
	if s.nc == nil {
		s.setClosedLocked(nil)
		s.mu.Unlock()
		return nil
	}

	s.closing = true
	s.setStateLocked(StateDraining)

	nc := s.nc
	shutdownTimeout := s.effective.Timeouts.ShutdownTimeout
	s.mu.Unlock()

	drainErr := drainConnection(ctx, nc, shutdownTimeout)
	if drainErr != nil {
		nc.Close()
	}

	s.mu.Lock()
	s.nc = nil
	s.js = nil
	s.kv = nil
	s.closing = false
	if drainErr != nil {
		s.setClosedLocked(drainErr)
	} else {
		s.setClosedLocked(nil)
	}
	s.mu.Unlock()

	if drainErr != nil {
		return &Error{Code: CodeShutdown, Op: "close", Message: "failed to drain NATS connection cleanly", Retryable: true, Err: drainErr}
	}
	return nil
}

func (s *runtimeSession) keyValue() (jetstream.KeyValue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.nc == nil || s.nc.Status() != nats.CONNECTED || s.kv == nil {
		return nil, &Error{Code: CodeDisconnected, Op: "key_value", Message: "client runtime is not connected", Retryable: true}
	}

	return s.kv, nil
}

func (s *runtimeSession) connectNATS() (*nats.Conn, error) {
	opts, err := s.buildNATSOptions()
	if err != nil {
		return nil, err
	}
	servers := strings.Join(s.effective.NATS.Servers, ",")
	return nats.Connect(servers, opts...)
}

func (s *runtimeSession) buildNATSOptions() ([]nats.Option, error) {
	ncfg := s.effective.NATS
	opts := []nats.Option{
		nats.Timeout(ncfg.ConnectTimeout),
		nats.RetryOnFailedConnect(ncfg.RetryOnFailedConnect),
		nats.MaxReconnects(ncfg.MaxReconnects),
		nats.ReconnectWait(ncfg.ReconnectWait),
		nats.ReconnectBufSize(ncfg.ReconnectBufSize),
		nats.DisconnectErrHandler(s.onDisconnect),
		nats.ReconnectHandler(s.onReconnect),
		nats.ClosedHandler(s.onClosed),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			s.onAsyncError(err)
		}),
	}

	clientName := strings.TrimSpace(ncfg.ClientName)
	if clientName == "" {
		clientName = strings.TrimSpace(s.effective.AgentName)
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

func (s *runtimeSession) newJetStream(nc *nats.Conn) (jetstream.JetStream, error) {
	opts := []jetstream.JetStreamOpt{
		jetstream.WithDefaultTimeout(s.effective.JetStream.DefaultTimeout),
	}
	if domain := strings.TrimSpace(s.effective.JetStream.Domain); domain != "" {
		return jetstream.NewWithDomain(nc, domain, opts...)
	}
	if prefix := strings.TrimSpace(s.effective.JetStream.APIPrefix); prefix != "" {
		return jetstream.NewWithAPIPrefix(nc, prefix, opts...)
	}
	return jetstream.New(nc, opts...)
}

func (s *runtimeSession) bindOrCreateKV(ctx context.Context, js jetstream.JetStream) (jetstream.KeyValue, error) {
	bucket := s.effective.KV.Bucket

	kv, err := js.KeyValue(ctx, bucket)
	if err == nil {
		return kv, nil
	}
	if !isBucketNotFound(err) {
		return nil, err
	}
	if !s.effective.KV.AutoCreateBucket {
		return nil, err
	}

	cfg := jetstream.KeyValueConfig{
		Bucket:       bucket,
		History:      s.effective.KV.History,
		TTL:          s.effective.KV.TTL,
		MaxValueSize: s.effective.KV.MaxValueSize,
		Replicas:     s.effective.KV.Replicas,
	}
	if strings.ToLower(s.effective.KV.Storage) == "memory" {
		cfg.Storage = jetstream.MemoryStorage
	} else {
		cfg.Storage = jetstream.FileStorage
	}
	return js.CreateKeyValue(ctx, cfg)
}

func (s *runtimeSession) onDisconnect(_ *nats.Conn, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closing {
		return
	}
	s.health.JetStreamReady = false
	s.health.KVReady = false
	if err != nil {
		s.health.LastError = err.Error()
	}
	s.setStateLocked(StateReconnecting)
}

func (s *runtimeSession) onReconnect(nc *nats.Conn) {
	s.mu.RLock()
	if s.closing {
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	go s.rebindAfterReconnect(nc)
}

func (s *runtimeSession) onClosed(_ *nats.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setClosedLocked(nil)
}

func (s *runtimeSession) rebindAfterReconnect(nc *nats.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), s.effective.Timeouts.KVTimeout)
	defer cancel()

	js, err := s.newJetStream(nc)
	if err != nil {
		s.mu.Lock()
		s.setDegradedLocked(err)
		s.mu.Unlock()
		s.onAsyncError(&Error{Code: CodeJetStreamFailed, Op: "reconnect_jetstream", Message: "failed to rebuild JetStream handle after reconnect", Retryable: true, Err: err})
		return
	}

	kv, err := s.bindOrCreateKV(ctx, js)
	if err != nil {
		s.mu.Lock()
		s.setDegradedLocked(err)
		s.mu.Unlock()
		s.onAsyncError(&Error{Code: CodeJetStreamFailed, Op: "reconnect_kv", Message: "failed to rebind KV bucket after reconnect", Retryable: true, Err: err})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closing {
		return
	}
	if s.nc == nil || s.nc != nc {
		return
	}
	s.js = js
	s.kv = kv
	s.setConnectedLocked(nc.ConnectedUrl(), true, true)
}

func (s *runtimeSession) onAsyncError(err error) {
	if err == nil {
		return
	}
	if s.hooks.Logger != nil {
		s.hooks.Logger.Error("session asynchronous error", "error", err)
	}
	if s.hooks.ErrorSink != nil {
		s.hooks.ErrorSink(err)
	}
}

func (s *runtimeSession) setStateLocked(state ConnectionState) {
	s.health.State = state
	if s.hooks.Metrics != nil {
		s.hooks.Metrics.SetConnectionState(string(state))
	}
}

func (s *runtimeSession) setConnectedLocked(url string, jsReady, kvReady bool) {
	s.health.ConnectedURL = url
	s.health.JetStreamReady = jsReady
	s.health.KVReady = kvReady
	s.health.LastError = ""
	if jsReady && kvReady {
		s.setStateLocked(StateConnected)
		return
	}
	s.setStateLocked(StateDegraded)
}

func (s *runtimeSession) setDegradedLocked(err error) {
	s.health.JetStreamReady = false
	s.health.KVReady = false
	if err != nil {
		s.health.LastError = err.Error()
	}
	s.setStateLocked(StateDegraded)
}

func (s *runtimeSession) setClosedLocked(err error) {
	s.health.ConnectedURL = ""
	s.health.JetStreamReady = false
	s.health.KVReady = false
	if err != nil {
		s.health.LastError = err.Error()
	}
	s.setStateLocked(StateClosed)
}

func normalizeRuntimeConfig(cfg Config) (Config, error) {
	const op = "normalize_runtime_config"

	out := cfg

	if out.NATS.ConnectTimeout < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "nats.connect_timeout cannot be negative", Retryable: false}
	}
	if out.NATS.ReconnectWait < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "nats.reconnect_wait cannot be negative", Retryable: false}
	}
	if out.NATS.MaxReconnects < -1 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "nats.max_reconnects must be -1 or greater", Retryable: false}
	}
	if out.NATS.ReconnectBufSize < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "nats.reconnect_buf_size cannot be negative", Retryable: false}
	}
	if out.JetStream.DefaultTimeout < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "jetstream.default_timeout cannot be negative", Retryable: false}
	}
	if out.Timeouts.PublishTimeout < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "timeouts.publish_timeout cannot be negative", Retryable: false}
	}
	if out.Timeouts.SubscribeTimeout < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "timeouts.subscribe_timeout cannot be negative", Retryable: false}
	}
	if out.Timeouts.KVTimeout < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "timeouts.kv_timeout cannot be negative", Retryable: false}
	}
	if out.Timeouts.ShutdownTimeout < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "timeouts.shutdown_timeout cannot be negative", Retryable: false}
	}
	if out.Timeouts.HandlerWarnAfter < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "timeouts.handler_warn_after cannot be negative", Retryable: false}
	}
	if out.Retry.PublishAttempts < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "retry.publish_attempts cannot be negative", Retryable: false}
	}
	if out.Retry.PublishBackoff < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "retry.publish_backoff cannot be negative", Retryable: false}
	}
	if out.KV.TTL < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "kv.ttl cannot be negative", Retryable: false}
	}
	if out.KV.MaxValueSize < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "kv.max_value_size cannot be negative", Retryable: false}
	}
	if out.KV.Replicas < 0 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "kv.replicas cannot be negative", Retryable: false}
	}

	servers := make([]string, 0, len(out.NATS.Servers))
	for _, server := range out.NATS.Servers {
		trimmed := strings.TrimSpace(server)
		if trimmed == "" {
			continue
		}
		servers = append(servers, trimmed)
	}
	if len(servers) == 0 {
		servers = []string{nats.DefaultURL}
	}
	out.NATS.Servers = servers

	if out.NATS.ConnectTimeout == 0 {
		out.NATS.ConnectTimeout = defaultConnectTimeout
	}
	if out.NATS.MaxReconnects == 0 {
		out.NATS.MaxReconnects = defaultMaxReconnects
	}
	if out.NATS.ReconnectWait == 0 {
		out.NATS.ReconnectWait = defaultReconnectWait
	}
	if out.NATS.ReconnectBufSize == 0 {
		out.NATS.ReconnectBufSize = nats.DefaultReconnectBufSize
	}
	if out.JetStream.DefaultTimeout == 0 {
		out.JetStream.DefaultTimeout = defaultJetStreamTimeout
	}
	if out.Timeouts.PublishTimeout == 0 {
		out.Timeouts.PublishTimeout = defaultPublishTimeout
	}
	if out.Timeouts.SubscribeTimeout == 0 {
		out.Timeouts.SubscribeTimeout = defaultSubscribeTimeout
	}
	if out.Timeouts.KVTimeout == 0 {
		out.Timeouts.KVTimeout = defaultKVTimeout
	}
	if out.Timeouts.ShutdownTimeout == 0 {
		out.Timeouts.ShutdownTimeout = defaultShutdownTimeout
	}
	if out.Timeouts.HandlerWarnAfter == 0 {
		out.Timeouts.HandlerWarnAfter = defaultHandlerWarnAfter
	}
	if out.Retry.PublishAttempts == 0 {
		out.Retry.PublishAttempts = defaultPublishAttempts
	}
	if out.Retry.PublishBackoff == 0 {
		out.Retry.PublishBackoff = defaultPublishBackoff
	}

	out.KV.Bucket = strings.TrimSpace(out.KV.Bucket)
	if out.KV.Bucket == "" {
		out.KV.Bucket = defaultKVBucket
	}
	out.KV.KeyPattern = strings.TrimSpace(out.KV.KeyPattern)
	if out.KV.KeyPattern == "" {
		out.KV.KeyPattern = defaultKVKey
	}
	if err := validateKeyPattern(out.KV.KeyPattern); err != nil {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: err.Error(), Retryable: false}
	}
	if out.KV.History == 0 {
		out.KV.History = defaultKVHistory
	}
	if out.KV.History > 64 {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "kv.history must be between 1 and 64", Retryable: false}
	}
	if out.KV.Replicas == 0 {
		out.KV.Replicas = defaultKVReplicas
	}
	storage := strings.ToLower(strings.TrimSpace(out.KV.Storage))
	if storage == "" {
		storage = defaultKVStorage
	}
	if storage != "file" && storage != "memory" {
		return Config{}, &Error{Code: CodeValidation, Op: op, Message: "kv.storage must be file or memory", Retryable: false}
	}
	out.KV.Storage = storage

	return out, nil
}

func validateKeyPattern(pattern string) error {
	if strings.ContainsAny(pattern, " \t\r\n") {
		return errors.New("kv.key_pattern cannot contain whitespace")
	}
	if strings.Count(pattern, "%s") != 1 {
		return errors.New("kv.key_pattern must contain exactly one %s placeholder")
	}
	residual := strings.ReplaceAll(pattern, "%s", "")
	if strings.Contains(residual, "%") {
		return errors.New("kv.key_pattern contains unsupported format directives")
	}
	return nil
}

func buildTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}
	out := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.InsecureSkipVerify, ServerName: cfg.ServerName}

	if strings.TrimSpace(cfg.CAFile) != "" {
		caPEM, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read tls ca file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("tls ca file contains no valid certificates")
		}
		out.RootCAs = pool
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
		out.Certificates = []tls.Certificate{cert}
	}

	return out, nil
}

func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok || timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func drainConnection(ctx context.Context, nc *nats.Conn, timeout time.Duration) error {
	if nc == nil {
		return nil
	}
	drainCtx, cancel := withTimeout(ctx, timeout)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- nc.Drain() }()

	select {
	case err := <-done:
		return err
	case <-drainCtx.Done():
		return drainCtx.Err()
	}
}

func isBucketNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, nats.ErrBucketNotFound) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "bucket not found") || strings.Contains(message, "stream not found")
}

type desiredKVStore struct {
	runtime   *runtimeSession
	errorSink func(error)
}

func newDesiredKVStore(runtime *runtimeSession, errorSink func(error)) *desiredKVStore {
	return &desiredKVStore{runtime: runtime, errorSink: errorSink}
}

func (s *desiredKVStore) StoreDesiredConfig(ctx context.Context, rec DesiredConfigRecord) (*StoredDesiredConfig, error) {
	if err := validateDesiredConfigRecord(rec); err != nil {
		return nil, err
	}

	effective := s.runtime.effectiveConfig()
	key, err := buildDesiredConfigKey(effective.KV.KeyPattern, rec.Target)
	if err != nil {
		return nil, err
	}

	kvHandle, err := s.runtime.keyValue()
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(rec)
	if err != nil {
		return nil, &Error{Code: CodeEncodeFailed, Op: "store_desired_config", Message: "failed to encode desired config", Retryable: false, Err: err}
	}

	opCtx, cancel := withTimeout(ctx, effective.Timeouts.KVTimeout)
	defer cancel()

	revision, err := kvHandle.Put(opCtx, key, payload)
	if err != nil {
		return nil, &Error{Code: CodeKVStoreFailed, Op: "store_desired_config", Message: "failed to write desired config to KV", Retryable: true, Err: err}
	}

	createdAt := rec.Timestamp
	entry, getErr := kvHandle.Get(opCtx, key)
	if getErr == nil {
		revision = entry.Revision()
		if !entry.Created().IsZero() {
			createdAt = entry.Created()
		}
	} else {
		s.reportAsync(&Error{Code: CodeKVReadFailed, Op: "store_desired_config_post_read", Message: "stored config metadata lookup failed", Retryable: true, Err: getErr})
	}

	return &StoredDesiredConfig{
		Record:    rec,
		Bucket:    effective.KV.Bucket,
		Key:       key,
		Revision:  revision,
		CreatedAt: createdAt,
	}, nil
}

func (s *desiredKVStore) LoadDesiredConfig(ctx context.Context, target string) (*StoredDesiredConfig, error) {
	effective := s.runtime.effectiveConfig()
	key, err := buildDesiredConfigKey(effective.KV.KeyPattern, target)
	if err != nil {
		return nil, err
	}

	kvHandle, err := s.runtime.keyValue()
	if err != nil {
		return nil, err
	}

	opCtx, cancel := withTimeout(ctx, effective.Timeouts.KVTimeout)
	defer cancel()

	entry, err := kvHandle.Get(opCtx, key)
	if err != nil {
		if isConfigNotFound(err) {
			return nil, &Error{Code: CodeConfigNotFound, Op: "load_desired_config", Key: key, Message: "desired config not found", Retryable: false, Err: err}
		}
		return nil, &Error{Code: CodeKVReadFailed, Op: "load_desired_config", Message: "failed to read desired config from KV", Retryable: true, Err: err}
	}

	var rec DesiredConfigRecord
	if err := json.Unmarshal(entry.Value(), &rec); err != nil {
		return nil, &Error{Code: CodeDecodeFailed, Op: "load_desired_config", Message: "failed to decode desired config", Retryable: false, Err: err}
	}
	if err := validateDesiredConfigRecord(rec); err != nil {
		return nil, err
	}

	stored := &StoredDesiredConfig{
		Record:   rec,
		Bucket:   entry.Bucket(),
		Key:      entry.Key(),
		Revision: entry.Revision(),
	}
	if !entry.Created().IsZero() {
		stored.CreatedAt = entry.Created()
	} else {
		stored.CreatedAt = rec.Timestamp
	}

	return stored, nil
}

func (s *desiredKVStore) WatchDesiredConfig(ctx context.Context, target string, handler DesiredConfigWatchHandler) (StopFunc, error) {
	if handler == nil {
		return nil, &Error{Code: CodeValidation, Op: "watch_desired_config", Message: "watch handler is required", Retryable: false}
	}

	effective := s.runtime.effectiveConfig()
	key, err := buildDesiredConfigKey(effective.KV.KeyPattern, target)
	if err != nil {
		return nil, err
	}

	kvHandle, err := s.runtime.keyValue()
	if err != nil {
		return nil, err
	}

	setupCtx, cancelSetup := withTimeout(ctx, effective.Timeouts.KVTimeout)
	defer cancelSetup()

	watcher, err := kvHandle.Watch(setupCtx, key)
	if err != nil {
		return nil, &Error{Code: CodeKVReadFailed, Op: "watch_desired_config", Message: "failed to start desired-config watch", Retryable: true, Err: err}
	}

	watchCtx, cancelWatch := context.WithCancel(context.Background())
	done := make(chan struct{})

	go s.consumeWatch(watchCtx, watcher, handler, done)

	if ctx != nil {
		go func() {
			<-ctx.Done()
			cancelWatch()
		}()
	}

	var once sync.Once
	return func() error {
		once.Do(func() {
			cancelWatch()
			watcher.Stop()
			<-done
		})
		return nil
	}, nil
}

func (s *desiredKVStore) consumeWatch(ctx context.Context, watcher jetstream.KeyWatcher, handler DesiredConfigWatchHandler, done chan struct{}) {
	defer close(done)
	updates := watcher.Updates()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-updates:
			if !ok {
				return
			}
			if entry == nil || len(entry.Value()) == 0 {
				continue
			}

			var rec DesiredConfigRecord
			if err := json.Unmarshal(entry.Value(), &rec); err != nil {
				s.reportAsync(&Error{Code: CodeKVReadFailed, Op: "watch_desired_config_decode", Message: "failed to decode desired-config watch entry", Retryable: true, Err: err})
				continue
			}
			if err := validateDesiredConfigRecord(rec); err != nil {
				s.reportAsync(err)
				continue
			}

			stored := StoredDesiredConfig{Record: rec, Bucket: entry.Bucket(), Key: entry.Key(), Revision: entry.Revision()}
			if !entry.Created().IsZero() {
				stored.CreatedAt = entry.Created()
			} else {
				stored.CreatedAt = rec.Timestamp
			}

			if err := handler(ctx, stored); err != nil {
				s.reportAsync(&Error{Code: CodeKVReadFailed, Op: "watch_desired_config_handler", Message: "desired-config watch handler returned error", Retryable: true, Err: err})
			}
		}
	}
}

func (s *desiredKVStore) reportAsync(err error) {
	if err == nil {
		return
	}
	if s.errorSink != nil {
		s.errorSink(err)
	}
}

func validateDesiredConfigRecord(rec DesiredConfigRecord) error {
	const op = "validate_desired_config_record"
	if strings.TrimSpace(rec.Version) == "" {
		return &Error{Code: CodeValidation, Op: op, Message: "version is required", Retryable: false}
	}
	if strings.TrimSpace(rec.RPCID) == "" {
		return &Error{Code: CodeValidation, Op: op, Message: "rpc_id is required", Retryable: false}
	}
	if strings.TrimSpace(rec.Target) == "" {
		return &Error{Code: CodeValidation, Op: op, Message: "target is required", Retryable: false}
	}
	if strings.TrimSpace(rec.UUID) == "" {
		return &Error{Code: CodeValidation, Op: op, Message: "uuid is required", Retryable: false}
	}
	if rec.Timestamp.IsZero() {
		return &Error{Code: CodeValidation, Op: op, Message: "timestamp is required", Retryable: false}
	}
	if len(rec.Payload) == 0 {
		return &Error{Code: CodeValidation, Op: op, Message: "payload is required", Retryable: false}
	}
	if !json.Valid(rec.Payload) {
		return &Error{Code: CodeValidation, Op: op, Message: "payload must contain valid JSON", Retryable: false}
	}
	if err := validateToken(rec.Target); err != nil {
		return err
	}
	return nil
}

func buildDesiredConfigKey(pattern, target string) (string, error) {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return "", &Error{Code: CodeValidation, Op: "build_desired_config_key", Message: "kv key pattern is required", Retryable: false}
	}
	if strings.ContainsAny(trimmed, " \t\r\n") {
		return "", &Error{Code: CodeValidation, Op: "build_desired_config_key", Message: "kv key pattern cannot contain whitespace", Retryable: false}
	}
	if strings.Count(trimmed, "%s") != 1 {
		return "", &Error{Code: CodeValidation, Op: "build_desired_config_key", Message: "kv key pattern must contain exactly one %s placeholder", Retryable: false}
	}
	residual := strings.ReplaceAll(trimmed, "%s", "")
	if strings.Contains(residual, "%") {
		return "", &Error{Code: CodeValidation, Op: "build_desired_config_key", Message: "kv key pattern contains unsupported format directives", Retryable: false}
	}
	if err := validateToken(target); err != nil {
		return "", err
	}
	return fmt.Sprintf(trimmed, target), nil
}

func validateToken(value string) error {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return &Error{Code: CodeValidation, Op: "validate_target", Message: "target contains unsupported characters", Retryable: false}
	}
	if strings.Contains(value, ".") {
		return &Error{Code: CodeValidation, Op: "validate_target", Message: "target cannot contain '.'", Retryable: false}
	}
	if strings.ContainsAny(value, "*>") {
		return &Error{Code: CodeValidation, Op: "validate_target", Message: "target cannot contain wildcard tokens", Retryable: false}
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return &Error{Code: CodeValidation, Op: "validate_target", Message: "target cannot contain whitespace", Retryable: false}
	}
	return nil
}

func isConfigNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, nats.ErrKeyNotFound) || errors.Is(err, nats.ErrKeyDeleted) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "key not found") || strings.Contains(msg, "key was deleted")
}
