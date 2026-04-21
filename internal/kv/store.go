package kv

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/nats-agent-core/internal/contract"
)

// RuntimeProvider exposes session runtime handles required by KV operations.
type RuntimeProvider interface {
	KeyValue() (jetstream.KeyValue, error)
	EffectiveConfig() agentcore.Config
}

// Store implements desired-config KV operations backed by JetStream KV.
type Store struct {
	runtime   RuntimeProvider
	errorSink func(error)
}

// NewStore creates a desired-config KV store bound to runtime state.
func NewStore(runtime RuntimeProvider, errorSink func(error)) (*Store, error) {
	if runtime == nil {
		return nil, &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "new_kv_store",
			Message:   "runtime provider is required",
			Retryable: false,
		}
	}
	return &Store{runtime: runtime, errorSink: errorSink}, nil
}

// StoreDesiredConfig stores a desired-config record in KV and returns metadata.
func (s *Store) StoreDesiredConfig(ctx context.Context, rec agentcore.DesiredConfigRecord) (*agentcore.StoredDesiredConfig, error) {
	if err := contract.ValidateDesiredConfigRecord(rec); err != nil {
		return nil, err
	}

	effective := s.runtime.EffectiveConfig()
	key, err := buildDesiredConfigKey(effective.KV.KeyPattern, rec.Target)
	if err != nil {
		return nil, err
	}

	kvHandle, err := s.runtime.KeyValue()
	if err != nil {
		return nil, err
	}

	payload, err := contract.EncodeDesiredConfigRecord(rec)
	if err != nil {
		return nil, err
	}

	opCtx, cancel := withTimeout(ctx, effective.Timeouts.KVTimeout)
	defer cancel()

	revision, err := kvHandle.Put(opCtx, key, payload)
	if err != nil {
		return nil, kvStoreError("store_desired_config", "failed to write desired config to KV", err)
	}

	createdAt := rec.Timestamp
	entry, getErr := kvHandle.Get(opCtx, key)
	if getErr == nil {
		revision = entry.Revision()
		if !entry.Created().IsZero() {
			createdAt = entry.Created()
		}
	} else {
		s.reportAsync(kvReadError("store_desired_config_post_read", "stored config metadata lookup failed", getErr))
	}

	stored := &agentcore.StoredDesiredConfig{
		Record:    rec,
		Bucket:    effective.KV.Bucket,
		Key:       key,
		Revision:  revision,
		CreatedAt: createdAt,
	}
	return stored, nil
}

// LoadDesiredConfig loads the latest desired-config record for a target.
func (s *Store) LoadDesiredConfig(ctx context.Context, target string) (*agentcore.StoredDesiredConfig, error) {
	effective := s.runtime.EffectiveConfig()
	key, err := buildDesiredConfigKey(effective.KV.KeyPattern, target)
	if err != nil {
		return nil, err
	}

	kvHandle, err := s.runtime.KeyValue()
	if err != nil {
		return nil, err
	}

	opCtx, cancel := withTimeout(ctx, effective.Timeouts.KVTimeout)
	defer cancel()

	entry, err := kvHandle.Get(opCtx, key)
	if err != nil {
		if isConfigNotFound(err) {
			return nil, &agentcore.Error{
				Code:      agentcore.CodeConfigNotFound,
				Op:        "load_desired_config",
				Key:       key,
				Message:   "desired config not found",
				Retryable: false,
				Err:       err,
			}
		}
		return nil, kvReadError("load_desired_config", "failed to read desired config from KV", err)
	}

	rec, err := contract.DecodeDesiredConfigRecord(entry.Value())
	if err != nil {
		return nil, err
	}

	stored := &agentcore.StoredDesiredConfig{
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

func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok || timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func isConfigNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, nats.ErrKeyNotFound) || errors.Is(err, nats.ErrKeyDeleted) || errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "key not found") || strings.Contains(message, "key was deleted")
}

func (s *Store) reportAsync(err error) {
	if err == nil {
		return
	}
	if s.errorSink != nil {
		s.errorSink(err)
	}
}
