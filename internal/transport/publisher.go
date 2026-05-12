package transport

import (
	"context"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/routerarchitects/nats-agent-core/internal/runtimeerr"
)

// ConnectionProvider resolves the active NATS connection used for publish operations.
type ConnectionProvider interface {
	Connection() (*nats.Conn, error)
}

// PublishMetrics is the optional publish metrics hook.
type PublishMetrics interface {
	IncPublish(kind, subject, result string)
	ObservePublishLatency(kind, subject string, d time.Duration)
}

// Publisher owns low-level publish and flush mechanics.
type Publisher struct {
	conn           ConnectionProvider
	publishTimeout func() time.Duration
	metrics        PublishMetrics
	publishFn      func(*nats.Conn, string, []byte) error
	flushFn        func(*nats.Conn, context.Context) error
}

// NewPublisher constructs a low-level transport publisher.
func NewPublisher(
	conn ConnectionProvider,
	publishTimeout func() time.Duration,
	metrics PublishMetrics,
) (*Publisher, error) {
	if conn == nil {
		return nil, &runtimeerr.Error{
			Code:      runtimeerr.CodeValidation,
			Op:        "new_publisher",
			Message:   "connection provider is required",
			Retryable: false,
		}
	}
	if publishTimeout == nil {
		publishTimeout = func() time.Duration { return 0 }
	}

	return &Publisher{
		conn:           conn,
		publishTimeout: publishTimeout,
		metrics:        metrics,
		publishFn: func(nc *nats.Conn, subject string, payload []byte) error {
			return nc.Publish(subject, payload)
		},
		flushFn: func(nc *nats.Conn, ctx context.Context) error {
			return nc.FlushWithContext(ctx)
		},
	}, nil
}

// Publish publishes an already-encoded payload and flushes within timeout/context limits.
func (p *Publisher) Publish(ctx context.Context, op, kind, subject string, payload []byte) error {
	if op == "" {
		op = "publish_payload"
	}
	if ctx == nil {
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeValidation,
			Op:        op,
			Subject:   subject,
			Message:   "context is required",
			Retryable: false,
		}
	}
	if err := ctx.Err(); err != nil {
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeValidation,
			Op:        op,
			Subject:   subject,
			Message:   "context is not usable",
			Retryable: false,
			Err:       err,
		}
	}
	if strings.TrimSpace(subject) == "" {
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeValidation,
			Op:        op,
			Message:   "publish subject is required",
			Retryable: false,
		}
	}
	if len(payload) == 0 {
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeValidation,
			Op:        op,
			Subject:   subject,
			Message:   "payload is required",
			Retryable: false,
		}
	}

	nc, err := p.conn.Connection()
	if err != nil {
		return err
	}

	started := time.Now()
	if err := p.publishFn(nc, subject, payload); err != nil {
		p.incPublish(kind, subject, "failure")
		return &runtimeerr.Error{
			Code:      runtimeerr.CodePublishFailed,
			Op:        op,
			Subject:   subject,
			Message:   "publish failed",
			Retryable: true,
			Err:       err,
		}
	}

	flushCtx, cancel := publishContext(ctx, p.publishTimeout())
	defer cancel()

	if err := p.flushFn(nc, flushCtx); err != nil {
		p.incPublish(kind, subject, "failure")
		return &runtimeerr.Error{
			Code:      runtimeerr.CodePublishFailed,
			Op:        op,
			Subject:   subject,
			Message:   "flush failed",
			Retryable: true,
			Err:       err,
		}
	}

	p.incPublish(kind, subject, "success")
	p.observePublishLatency(kind, subject, time.Since(started))
	return nil
}

func publishContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithCancel(context.Background())
	}
	if _, ok := ctx.Deadline(); ok || timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func (p *Publisher) incPublish(kind, subject, result string) {
	if p.metrics == nil {
		return
	}
	p.metrics.IncPublish(kind, subject, result)
}

func (p *Publisher) observePublishLatency(kind, subject string, d time.Duration) {
	if p.metrics == nil {
		return
	}
	p.metrics.ObservePublishLatency(kind, subject, d)
}
