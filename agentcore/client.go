package agentcore

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ConfigureHandler handles configure notifications for a target.
type ConfigureHandler func(context.Context, ConfigureNotification) error

// ActionHandler handles action commands for a target and action name.
type ActionHandler func(context.Context, ActionCommand) error

// ResultHandler handles result messages for a target.
type ResultHandler func(context.Context, ResultEnvelope) error

// StatusHandler handles status messages for a target.
type StatusHandler func(context.Context, StatusEnvelope) error

// DesiredConfigWatchHandler handles desired-config watch updates.
type DesiredConfigWatchHandler func(context.Context, StoredDesiredConfig) error

// StopFunc stops a watch registration created by a public API.
type StopFunc func() error

// SubscriptionOption configures a public subscription registration.
type SubscriptionOption func(*SubscriptionOptions)

// SubscriptionOptions contains public subscription registration settings.
type SubscriptionOptions struct {
	QueueGroup string
}

type clientOptions struct {
	logger    Logger
	metrics   Metrics
	now       func() time.Time
	errorSink func(error)
}

// Option applies an optional public client setting during construction.
type Option func(*clientOptions) error

// WithLogger injects a logger into the client.
func WithLogger(l Logger) Option {
	return func(opts *clientOptions) error {
		opts.logger = l
		return nil
	}
}

// WithMetrics injects metrics hooks into the client.
func WithMetrics(m Metrics) Option {
	return func(opts *clientOptions) error {
		opts.metrics = m
		return nil
	}
}

// WithClock overrides the clock used by bootstrap defaults.
func WithClock(now func() time.Time) Option {
	return func(opts *clientOptions) error {
		if now == nil {
			return &Error{Code: CodeValidation, Op: "with_clock", Message: "clock function is nil"}
		}
		opts.now = now
		return nil
	}
}

// WithErrorSink injects a best-effort async error sink hook.
func WithErrorSink(fn func(error)) Option {
	return func(opts *clientOptions) error {
		opts.errorSink = fn
		return nil
	}
}

// Client is the public facade used by agent processes.
type Client struct {
	mu      sync.RWMutex
	cfg     Config
	options clientOptions

	session *runtimeSession
	kv      *desiredKVStore

	nextWatchID uint64
	watches     map[uint64]StopFunc
}

// New validates public options and constructs a bootstrap client facade.
func New(cfg Config, opts ...Option) (*Client, error) {
	options := clientOptions{
		now: time.Now,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&options); err != nil {
			return nil, err
		}
	}

	if options.logger == nil {
		options.logger = cfg.Observe.Logger
	}
	if options.metrics == nil {
		options.metrics = cfg.Observe.Metrics
	}

	runtime, err := newRuntimeSession(cfg, runtimeHooks{
		Logger:    options.logger,
		Metrics:   options.metrics,
		ErrorSink: options.errorSink,
	})
	if err != nil {
		return nil, err
	}

	store := newDesiredKVStore(runtime, options.errorSink)

	return &Client{
		cfg:     cfg,
		options: options,
		session: runtime,
		kv:      store,
		watches: make(map[uint64]StopFunc),
	}, nil
}

// Config returns the bootstrap configuration snapshot.
func (c *Client) Config() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cfg
}

// Start begins the client lifecycle.
func (c *Client) Start(ctx context.Context) error {
	return c.session.start(ctx)
}

// Close ends the client lifecycle with watch cleanup and connection drain.
func (c *Client) Close(ctx context.Context) error {
	watchErr := c.stopAllWatches()
	sessionErr := c.session.close(ctx)

	if watchErr != nil && sessionErr == nil {
		return &Error{
			Code:      CodeShutdown,
			Op:        "close_stop_watches",
			Message:   "failed to stop one or more desired-config watches",
			Retryable: false,
			Err:       watchErr,
		}
	}
	if watchErr != nil && sessionErr != nil {
		return &Error{
			Code:      CodeShutdown,
			Op:        "close",
			Message:   "close failed with watch-stop and session shutdown errors",
			Retryable: true,
			Err:       errors.Join(watchErr, sessionErr),
		}
	}
	return sessionErr
}

// Health returns the latest public health snapshot.
func (c *Client) Health() HealthSnapshot {
	if c.session == nil {
		return HealthSnapshot{State: StateNew}
	}
	return c.session.healthSnapshot()
}

// SubmitConfigure accepts a configure command for later-phase transport logic.
func (c *Client) SubmitConfigure(ctx context.Context, cmd ConfigureCommand) (*SubmissionAck, error) {
	_ = ctx
	_ = cmd

	return nil, &Error{
		Code:      CodeNotImplemented,
		Op:        "submit_configure",
		Message:   "SubmitConfigure is not implemented in bootstrap phase",
		Retryable: false,
	}
}

// SubmitAction accepts an action command for later-phase transport logic.
func (c *Client) SubmitAction(ctx context.Context, cmd ActionCommand) (*SubmissionAck, error) {
	_ = ctx
	_ = cmd

	return nil, &Error{
		Code:      CodeNotImplemented,
		Op:        "submit_action",
		Message:   "SubmitAction is not implemented in bootstrap phase",
		Retryable: false,
	}
}

// PublishResult publishes a result envelope in later phases.
func (c *Client) PublishResult(ctx context.Context, msg ResultEnvelope) error {
	_ = ctx
	_ = msg

	return &Error{
		Code:      CodeNotImplemented,
		Op:        "publish_result",
		Message:   "PublishResult is not implemented in bootstrap phase",
		Retryable: false,
	}
}

// PublishStatus publishes a status envelope in later phases.
func (c *Client) PublishStatus(ctx context.Context, msg StatusEnvelope) error {
	_ = ctx
	_ = msg

	return &Error{
		Code:      CodeNotImplemented,
		Op:        "publish_status",
		Message:   "PublishStatus is not implemented in bootstrap phase",
		Retryable: false,
	}
}

// StoreDesiredConfig writes desired configuration to JetStream KV.
func (c *Client) StoreDesiredConfig(ctx context.Context, rec DesiredConfigRecord) (*StoredDesiredConfig, error) {
	return c.kv.StoreDesiredConfig(ctx, rec)
}

// LoadDesiredConfig loads desired configuration from JetStream KV.
func (c *Client) LoadDesiredConfig(ctx context.Context, target string) (*StoredDesiredConfig, error) {
	return c.kv.LoadDesiredConfig(ctx, target)
}

// WatchDesiredConfig registers a desired-config watch scoped to a single target.
func (c *Client) WatchDesiredConfig(ctx context.Context, target string, handler DesiredConfigWatchHandler) (StopFunc, error) {
	stop, err := c.kv.WatchDesiredConfig(ctx, target, handler)
	if err != nil {
		return nil, err
	}
	return c.trackWatch(stop), nil
}

// StartupReconcile loads latest desired state during recovery.
func (c *Client) StartupReconcile(ctx context.Context, target string) (*StoredDesiredConfig, error) {
	return c.LoadDesiredConfig(ctx, target)
}

// RegisterConfigureHandler registers a configure notification handler.
func (c *Client) RegisterConfigureHandler(target string, handler ConfigureHandler, opts ...SubscriptionOption) error {
	_ = target
	_ = handler
	_ = opts

	return &Error{
		Code:      CodeNotImplemented,
		Op:        "register_configure_handler",
		Message:   "RegisterConfigureHandler is not implemented in bootstrap phase",
		Retryable: false,
	}
}

// RegisterActionHandler registers a target/action handler.
func (c *Client) RegisterActionHandler(target, action string, handler ActionHandler, opts ...SubscriptionOption) error {
	_ = target
	_ = action
	_ = handler
	_ = opts

	return &Error{
		Code:      CodeNotImplemented,
		Op:        "register_action_handler",
		Message:   "RegisterActionHandler is not implemented in bootstrap phase",
		Retryable: false,
	}
}

// RegisterResultHandler registers a result handler.
func (c *Client) RegisterResultHandler(target string, handler ResultHandler, opts ...SubscriptionOption) error {
	_ = target
	_ = handler
	_ = opts

	return &Error{
		Code:      CodeNotImplemented,
		Op:        "register_result_handler",
		Message:   "RegisterResultHandler is not implemented in bootstrap phase",
		Retryable: false,
	}
}

// RegisterStatusHandler registers a status handler.
func (c *Client) RegisterStatusHandler(target string, handler StatusHandler, opts ...SubscriptionOption) error {
	_ = target
	_ = handler
	_ = opts

	return &Error{
		Code:      CodeNotImplemented,
		Op:        "register_status_handler",
		Message:   "RegisterStatusHandler is not implemented in bootstrap phase",
		Retryable: false,
	}
}

func (c *Client) trackWatch(stop StopFunc) StopFunc {
	id := atomic.AddUint64(&c.nextWatchID, 1)

	c.mu.Lock()
	c.watches[id] = stop
	c.mu.Unlock()

	var once sync.Once
	return func() error {
		var stopErr error
		once.Do(func() {
			c.mu.Lock()
			stored := c.watches[id]
			delete(c.watches, id)
			c.mu.Unlock()
			if stored != nil {
				stopErr = stored()
			}
		})
		return stopErr
	}
}

func (c *Client) stopAllWatches() error {
	c.mu.Lock()
	stops := make([]StopFunc, 0, len(c.watches))
	for id, stop := range c.watches {
		_ = id
		stops = append(stops, stop)
	}
	c.watches = make(map[uint64]StopFunc)
	c.mu.Unlock()

	var joined error
	for _, stop := range stops {
		if stop == nil {
			continue
		}
		if err := stop(); err != nil {
			if joined == nil {
				joined = err
			} else {
				joined = errors.Join(joined, err)
			}
		}
	}

	return joined
}
