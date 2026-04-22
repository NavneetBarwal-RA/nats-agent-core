package kv

import (
	"context"
	"sync"

	"github.com/nats-io/nats.go/jetstream"
)

// WatchDesiredConfig watches desired-config updates for a specific target.
func (s *Store) WatchDesiredConfig(ctx context.Context, target string, handler WatchHandler) (StopFunc, error) {
	if handler == nil {
		return nil, validationError("watch_desired_config", "watch handler is required")
	}

	key, err := buildDesiredConfigKey(s.runtime.DesiredConfigKeyPattern(), target)
	if err != nil {
		return nil, err
	}

	kvHandle, err := s.runtime.KeyValue()
	if err != nil {
		return nil, err
	}

	setupCtx, cancelSetup := withTimeout(ctx, s.runtime.KVTimeout())
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

func (s *Store) consumeWatch(ctx context.Context, watcher jetstream.KeyWatcher, handler WatchHandler) {
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

			rec, err := decodeDesiredConfigRecord(entry.Value())
			if err != nil {
				s.reportAsync(kvReadError("watch_desired_config_decode", "failed to decode desired-config watch entry", err))
				continue
			}

			stored := StoredDesiredConfig{
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
