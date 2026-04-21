package kv

import (
	"context"
	"sync"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/nats-agent-core/internal/contract"
)

// WatchDesiredConfig watches desired-config updates for a specific target.
func (s *Store) WatchDesiredConfig(ctx context.Context, target string, handler agentcore.DesiredConfigWatchHandler) (agentcore.StopFunc, error) {
	if handler == nil {
		return nil, &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "watch_desired_config",
			Message:   "watch handler is required",
			Retryable: false,
		}
	}

	effective := s.runtime.EffectiveConfig()
	key, err := buildDesiredConfigKey(effective.KV.KeyPattern, target)
	if err != nil {
		return nil, err
	}

	kvHandle, err := s.runtime.KeyValue()
	if err != nil {
		return nil, err
	}

	setupCtx, cancelSetup := withTimeout(ctx, effective.Timeouts.KVTimeout)
	defer cancelSetup()

	watcher, err := kvHandle.Watch(setupCtx, key)
	if err != nil {
		return nil, kvReadError("watch_desired_config", "failed to start desired-config watch", err)
	}

	watchCtx, cancelWatch := context.WithCancel(context.Background())
	done := make(chan struct{})

	go s.consumeWatch(watchCtx, watcher, handler)
	go func() {
		<-watchCtx.Done()
		close(done)
	}()

	if ctx != nil {
		go func() {
			<-ctx.Done()
			cancelWatch()
		}()
	}

	var once sync.Once
	stop := func() error {
		once.Do(func() {
			cancelWatch()
			watcher.Stop()
			<-done
		})
		return nil
	}

	return stop, nil
}

func (s *Store) consumeWatch(ctx context.Context, watcher jetstream.KeyWatcher, handler agentcore.DesiredConfigWatchHandler) {
	updates := watcher.Updates()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-updates:
			if !ok {
				return
			}
			if entry == nil {
				continue
			}
			if len(entry.Value()) == 0 {
				continue
			}

			rec, err := contract.DecodeDesiredConfigRecord(entry.Value())
			if err != nil {
				s.reportAsync(kvReadError("watch_desired_config_decode", "failed to decode desired-config watch entry", err))
				continue
			}

			stored := agentcore.StoredDesiredConfig{
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

			if err := handler(ctx, stored); err != nil {
				s.reportAsync(kvReadError("watch_desired_config_handler", "desired-config watch handler returned error", err))
			}
		}
	}
}
