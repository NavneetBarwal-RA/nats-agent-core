package registry

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/nats-io/nats.go"
	"github.com/routerarchitects/nats-agent-core/internal/runtimeerr"
)

// Registry stores subscription intent and runtime activation state.
type Registry struct {
	mu      sync.RWMutex
	nextID  uint64
	entries map[string]*entry
	byKey   map[string]string
}

// New creates an empty subscription registry.
func New() *Registry {
	return &Registry{
		entries: make(map[string]*entry),
		byKey:   make(map[string]string),
	}
}

// Add stores a new registration intent.
func (r *Registry) Add(spec AddSpec) (Snapshot, error) {
	if err := validateAddSpec(spec); err != nil {
		return Snapshot{}, err
	}

	regID := fmt.Sprintf("sub-%d", atomic.AddUint64(&r.nextID, 1))
	key := registrationKey(spec.Kind, spec.Subject, spec.QueueGroup)

	r.mu.Lock()
	defer r.mu.Unlock()

	if existingID, ok := r.byKey[key]; ok {
		return Snapshot{}, &runtimeerr.Error{
			Code:      runtimeerr.CodeRegistryConflict,
			Op:        "registry_add",
			Subject:   spec.Subject,
			Message:   "subscription registration already exists",
			Retryable: false,
			Err:       fmt.Errorf("conflicts with existing registration %s", existingID),
		}
	}

	e := &entry{
		ID:         regID,
		Key:        key,
		Kind:       spec.Kind,
		Target:     spec.Target,
		Action:     spec.Action,
		Subject:    spec.Subject,
		QueueGroup: spec.QueueGroup,
		Callback:   spec.Callback,
	}
	r.entries[regID] = e
	r.byKey[key] = regID

	return snapshotFromEntry(e), nil
}

// GetActivationRecord returns the activation state for one registration.
func (r *Registry) GetActivationRecord(id string) (ActivationRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[id]
	if !ok {
		return ActivationRecord{}, false
	}
	return activationFromEntry(e), true
}

// ListActivations returns all activation snapshots.
func (r *Registry) ListActivations() []ActivationRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ActivationRecord, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, activationFromEntry(e))
	}
	return out
}

// MarkActive stores the active runtime handle after successful subscription.
func (r *Registry) MarkActive(id string, sub *nats.Subscription) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[id]
	if !ok {
		return
	}
	if sub == nil {
		e.ActiveSub = nil
		e.Active = false
		return
	}
	e.ActiveSub = sub
	e.Active = e.ActiveSub != nil
	e.LastActivationErr = ""
}

// MarkInactive marks a registration inactive and stores the latest activation error.
func (r *Registry) MarkInactive(id string, activationErr error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[id]
	if !ok {
		return
	}
	e.ActiveSub = nil
	e.Active = false
	if activationErr != nil {
		e.LastActivationErr = activationErr.Error()
	}
}

// ClearActiveHandles detaches all active handles and returns them for cleanup.
func (r *Registry) ClearActiveHandles() []ActiveHandle {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]ActiveHandle, 0)
	for _, e := range r.entries {
		if e.ActiveSub == nil {
			e.Active = false
			continue
		}
		out = append(out, ActiveHandle{
			ID:      e.ID,
			Subject: e.Subject,
			Sub:     e.ActiveSub,
		})
		e.ActiveSub = nil
		e.Active = false
	}
	return out
}

// Remove deletes a registration entry and detaches any active handle.
func (r *Registry) Remove(id string) (ActiveHandle, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	e, ok := r.entries[id]
	if !ok {
		return ActiveHandle{}, false
	}

	handle := ActiveHandle{
		ID:      e.ID,
		Subject: e.Subject,
		Sub:     e.ActiveSub,
	}

	delete(r.entries, id)
	delete(r.byKey, e.Key)

	return handle, true
}

// List returns read-only snapshots for diagnostics.
func (r *Registry) List() []Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Snapshot, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, snapshotFromEntry(e))
	}
	return out
}

// Counts returns registered and active subscription counts.
func (r *Registry) Counts() (registered int, active int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	registered = len(r.entries)
	for _, e := range r.entries {
		if e.ActiveSub != nil && e.Active {
			active++
		}
	}
	return registered, active
}

func validateAddSpec(spec AddSpec) error {
	if strings.TrimSpace(string(spec.Kind)) == "" {
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeValidation,
			Op:        "registry_add",
			Message:   "registration kind is required",
			Retryable: false,
		}
	}
	if strings.TrimSpace(spec.Subject) == "" {
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeValidation,
			Op:        "registry_add",
			Message:   "registration subject is required",
			Retryable: false,
		}
	}
	if spec.Callback == nil {
		return &runtimeerr.Error{
			Code:      runtimeerr.CodeValidation,
			Op:        "registry_add",
			Message:   "registration callback is required",
			Retryable: false,
		}
	}
	return nil
}

func registrationKey(kind Kind, subject, queueGroup string) string {
	return string(kind) + "|" + strings.TrimSpace(subject) + "|" + strings.TrimSpace(queueGroup)
}

func activationFromEntry(e *entry) ActivationRecord {
	if e == nil {
		return ActivationRecord{}
	}
	return ActivationRecord{
		ID:         e.ID,
		Kind:       e.Kind,
		Subject:    e.Subject,
		QueueGroup: e.QueueGroup,
		Callback:   e.Callback,
		ActiveSub:  e.ActiveSub,
		Active:     e.Active,
	}
}

func snapshotFromEntry(e *entry) Snapshot {
	if e == nil {
		return Snapshot{}
	}
	return Snapshot{
		ID:                e.ID,
		Kind:              e.Kind,
		Target:            e.Target,
		Action:            e.Action,
		Subject:           e.Subject,
		QueueGroup:        e.QueueGroup,
		Active:            e.Active,
		LastActivationErr: e.LastActivationErr,
	}
}
