package transport

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/nats-agent-core/internal/contract"
	"github.com/routerarchitects/nats-agent-core/internal/subjects"
)

const (
	defaultKVBucketPattern = "cfg_desired"
	defaultKVKeyPattern    = "desired.%s"
)

// DesiredConfigStore stores desired configuration records durably.
type DesiredConfigStore interface {
	StoreDesiredConfig(ctx context.Context, record agentcore.DesiredConfigRecord) (*agentcore.StoredDesiredConfig, error)
}

// ConfigurePaths centralizes the configure store-then-notify publish path.
type ConfigurePaths struct {
	store        DesiredConfigStore
	publisher    Publisher
	publishPaths *PublishPaths
	now          func() time.Time
	kvBucket     string
	kvKeyPattern string
}

// NewConfigurePaths creates a configure path helper with explicit dependencies.
func NewConfigurePaths(
	store DesiredConfigStore,
	publisher Publisher,
	subjectBuilder *subjects.Builder,
	kvCfg agentcore.KVConfig,
	now func() time.Time,
) (*ConfigurePaths, error) {
	if store == nil {
		return nil, &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "new_configure_paths",
			Message:   "desired config store is required",
			Retryable: false,
		}
	}
	if publisher == nil {
		return nil, &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "new_configure_paths",
			Message:   "publisher is required",
			Retryable: false,
		}
	}
	publishPaths, err := NewPublishPaths(subjectBuilder)
	if err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}

	bucket := strings.TrimSpace(kvCfg.Bucket)
	if bucket == "" {
		bucket = defaultKVBucketPattern
	}

	keyPattern := strings.TrimSpace(kvCfg.KeyPattern)
	if keyPattern == "" {
		keyPattern = defaultKVKeyPattern
	}

	return &ConfigurePaths{
		store:        store,
		publisher:    publisher,
		publishPaths: publishPaths,
		now:          now,
		kvBucket:     bucket,
		kvKeyPattern: keyPattern,
	}, nil
}

// SubmitConfigure executes the shared configure flow:
// 1) validate configure command
// 2) store desired config durably
// 3) publish lightweight configure notification
func (c *ConfigurePaths) SubmitConfigure(ctx context.Context, cmd agentcore.ConfigureCommand) (*agentcore.SubmissionAck, error) {
	if err := contract.ValidateConfigureCommand(cmd); err != nil {
		return nil, err
	}

	record := agentcore.DesiredConfigRecord{
		Version:   cmd.Version,
		RPCID:     cmd.RPCID,
		Target:    cmd.Target,
		UUID:      cmd.UUID,
		Payload:   cmd.Payload,
		Timestamp: cmd.Timestamp,
	}

	stored, err := c.store.StoreDesiredConfig(ctx, record)
	if err != nil {
		return nil, &agentcore.Error{
			Code:      agentcore.CodeKVStoreFailed,
			Op:        "submit_configure_store",
			Message:   "failed to store desired config",
			Retryable: true,
			Err:       err,
		}
	}
	if stored == nil {
		return nil, &agentcore.Error{
			Code:      agentcore.CodeKVStoreFailed,
			Op:        "submit_configure_store",
			Message:   "desired config store returned nil result",
			Retryable: true,
		}
	}

	bucket := stored.Bucket
	if bucket == "" {
		bucket = c.kvBucket
	}

	key := stored.Key
	if key == "" {
		key, err = buildKVKey(c.kvKeyPattern, cmd.Target)
		if err != nil {
			return nil, err
		}
	}
	configureSubject, err := c.publishPaths.subjects.ConfigureSubject(cmd.Target)
	if err != nil {
		return nil, err
	}

	notification := agentcore.ConfigureNotification{
		Version:     cmd.Version,
		RPCID:       cmd.RPCID,
		Target:      cmd.Target,
		CommandType: "configure",
		UUID:        cmd.UUID,
		KVBucket:    bucket,
		KVKey:       key,
		Timestamp:   c.now(),
	}

	if err := c.publishPaths.PublishConfigureNotification(ctx, c.publisher, notification); err != nil {
		return nil, err
	}

	return &agentcore.SubmissionAck{
		Accepted:   true,
		RPCID:      cmd.RPCID,
		Target:     cmd.Target,
		Subject:    configureSubject,
		AcceptedAt: notification.Timestamp,
		KVBucket:   bucket,
		KVKey:      key,
		KVRevision: stored.Revision,
	}, nil
}

func buildKVKey(pattern, target string) (string, error) {
	trimmedPattern := strings.TrimSpace(pattern)
	if trimmedPattern == "" {
		return "", &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "build_kv_key",
			Message:   "kv key pattern is required",
			Retryable: false,
		}
	}
	// KV key patterns are validated as storage keys and intentionally remain
	// separate from NATS subject-pattern validation rules.
	if strings.ContainsAny(trimmedPattern, " \t\r\n") {
		return "", &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "build_kv_key",
			Message:   "kv key pattern cannot contain whitespace",
			Retryable: false,
		}
	}
	if strings.Count(trimmedPattern, "%s") != 1 {
		return "", &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "build_kv_key",
			Message:   "kv key pattern must contain exactly one %s placeholder",
			Retryable: false,
		}
	}
	residual := strings.ReplaceAll(trimmedPattern, "%s", "")
	if strings.Contains(residual, "%") {
		return "", &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "build_kv_key",
			Message:   "kv key pattern contains unsupported format directives",
			Retryable: false,
		}
	}
	if err := subjects.ValidateTarget(target); err != nil {
		return "", err
	}
	return fmt.Sprintf(trimmedPattern, target), nil
}
