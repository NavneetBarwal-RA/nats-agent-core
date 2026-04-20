package transport

import (
	"context"
	"time"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/nats-agent-core/internal/contract"
	"github.com/routerarchitects/nats-agent-core/internal/subjects"
)

// Publisher is the shared publish dependency used by transport wrappers.
type Publisher interface {
	Publish(ctx context.Context, subject string, payload []byte) error
}

// PublishPaths centralizes publish wrappers for shared message types.
type PublishPaths struct {
	subjects *subjects.Builder
	now      func() time.Time
}

// NewPublishPaths creates publish wrappers with a validated subject builder.
func NewPublishPaths(builder *subjects.Builder) (*PublishPaths, error) {
	if builder == nil {
		return nil, &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "new_publish_paths",
			Message:   "subject builder is required",
			Retryable: false,
		}
	}
	return &PublishPaths{
		subjects: builder,
		now:      time.Now,
	}, nil
}

// PublishConfigureNotification publishes a lightweight configure notification.
func (p *PublishPaths) PublishConfigureNotification(ctx context.Context, publisher Publisher, msg agentcore.ConfigureNotification) error {
	subject, err := p.subjects.ConfigureSubject(msg.Target)
	if err != nil {
		return err
	}
	payload, err := contract.EncodeConfigureNotification(msg)
	if err != nil {
		return err
	}
	return publishEncoded(ctx, publisher, "publish_configure_notification", subject, payload)
}

// SubmitAction publishes an action command on the centralized action subject path.
func (p *PublishPaths) SubmitAction(ctx context.Context, publisher Publisher, cmd agentcore.ActionCommand) (*agentcore.SubmissionAck, error) {
	subject, err := p.subjects.ActionSubject(cmd.Target, cmd.Action)
	if err != nil {
		return nil, err
	}
	payload, err := contract.EncodeActionCommand(cmd)
	if err != nil {
		return nil, err
	}
	if err := publishEncoded(ctx, publisher, "submit_action_publish", subject, payload); err != nil {
		return nil, err
	}

	acceptedAt := time.Now()
	if p.now != nil {
		acceptedAt = p.now()
	}

	return &agentcore.SubmissionAck{
		Accepted:   true,
		RPCID:      cmd.RPCID,
		Target:     cmd.Target,
		Subject:    subject,
		AcceptedAt: acceptedAt,
	}, nil
}

// PublishResult publishes a result envelope through the shared result path.
func (p *PublishPaths) PublishResult(ctx context.Context, publisher Publisher, msg agentcore.ResultEnvelope) error {
	subject, err := p.subjects.ResultSubject(msg.Target)
	if err != nil {
		return err
	}
	payload, err := contract.EncodeResultEnvelope(msg)
	if err != nil {
		return err
	}
	return publishEncoded(ctx, publisher, "publish_result", subject, payload)
}

// PublishStatus publishes a status envelope through the shared status path.
func (p *PublishPaths) PublishStatus(ctx context.Context, publisher Publisher, msg agentcore.StatusEnvelope) error {
	subject, err := p.subjects.StatusSubject(msg.Target)
	if err != nil {
		return err
	}
	payload, err := contract.EncodeStatusEnvelope(msg)
	if err != nil {
		return err
	}
	return publishEncoded(ctx, publisher, "publish_status", subject, payload)
}

func publishEncoded(ctx context.Context, publisher Publisher, op, subject string, payload []byte) error {
	if publisher == nil {
		return &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        op,
			Subject:   subject,
			Message:   "publisher is required",
			Retryable: false,
		}
	}
	if err := publisher.Publish(ctx, subject, payload); err != nil {
		return &agentcore.Error{
			Code:      agentcore.CodePublishFailed,
			Op:        op,
			Subject:   subject,
			Message:   "publish failed",
			Retryable: true,
			Err:       err,
		}
	}
	return nil
}
