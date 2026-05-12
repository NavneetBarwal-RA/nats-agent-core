package registry

import (
	"errors"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/routerarchitects/nats-agent-core/internal/runtimeerr"
)

func requireRegistryRuntimeError(t *testing.T, err error, wantCode runtimeerr.Code, wantOp string, wantMsgPart string) *runtimeerr.Error {
	t.Helper()

	if err == nil {
		t.Fatal("expected non-nil error")
	}

	var got *runtimeerr.Error
	if !errors.As(err, &got) {
		t.Fatalf("expected *runtimeerr.Error, got %T", err)
	}
	if got.Code != wantCode {
		t.Fatalf("expected error code %q, got %q", wantCode, got.Code)
	}
	if got.Op != wantOp {
		t.Fatalf("expected error op %q, got %q", wantOp, got.Op)
	}
	if wantMsgPart != "" && !strings.Contains(got.Message, wantMsgPart) {
		t.Fatalf("expected error message to contain %q, got %q", wantMsgPart, got.Message)
	}
	return got
}

/*
TC-REGISTRY-001
Type: Positive
Title: Add stores configure action result and status registration intents
Summary:
Verifies that subscription registry accepts all handler kinds and stores
intent metadata required for later activation and restore behavior.

Validates:
  - Add succeeds for configure action result and status entries
  - registry snapshot count matches number of added entries
  - target action subject and queue-group metadata are preserved
*/
func TestAddStoresAllRegistrationKinds(t *testing.T) {
	r := New()
	callback := func(*nats.Msg) {}

	entries := []AddSpec{
		{Kind: KindConfigure, Target: "vyos", Subject: "cmd.configure.vyos", QueueGroup: "cfg-workers", Callback: callback},
		{Kind: KindAction, Target: "vyos", Action: "trace", Subject: "cmd.action.vyos.trace", Callback: callback},
		{Kind: KindResult, Target: "vyos", Subject: "result.vyos", Callback: callback},
		{Kind: KindStatus, Target: "vyos", Subject: "status.vyos", Callback: callback},
	}

	for _, spec := range entries {
		if _, err := r.Add(spec); err != nil {
			t.Fatalf("expected nil add error for kind %q, got %v", spec.Kind, err)
		}
	}

	snapshots := r.List()
	if len(snapshots) != 4 {
		t.Fatalf("expected %d snapshots, got %d", 4, len(snapshots))
	}

	registered, active := r.Counts()
	if registered != 4 {
		t.Fatalf("expected registered count %d, got %d", 4, registered)
	}
	if active != 0 {
		t.Fatalf("expected active count %d before activation, got %d", 0, active)
	}
}

/*
TC-REGISTRY-002
Type: Negative
Title: Add rejects duplicate registration key entries
Summary:
Verifies that duplicate kind+subject+queue-group registration attempts are
rejected with a registry conflict error.

Validates:
  - second duplicate Add fails
  - returned error code is CodeRegistryConflict
  - returned error op is registry_add
*/
func TestAddRejectsDuplicateRegistrationKey(t *testing.T) {
	r := New()
	spec := AddSpec{
		Kind:       KindResult,
		Target:     "vyos",
		Subject:    "result.vyos",
		QueueGroup: "",
		Callback:   func(*nats.Msg) {},
	}

	if _, err := r.Add(spec); err != nil {
		t.Fatalf("expected first add to succeed, got %v", err)
	}
	_, err := r.Add(spec)
	requireRegistryRuntimeError(t, err, runtimeerr.CodeRegistryConflict, "registry_add", "subscription registration already exists")
}

/*
TC-REGISTRY-003
Type: Positive
Title: MarkActive and ClearActiveHandles update activation state correctly
Summary:
Verifies that active subscription handles can be attached and detached while
registry counts and snapshots reflect active/inactive transitions.

Validates:
  - MarkActive sets active count for the entry
  - ClearActiveHandles returns attached handle and marks entry inactive
  - active count returns to zero after clear
*/
func TestMarkActiveAndClearActiveHandlesTransitionsState(t *testing.T) {
	r := New()
	snap, err := r.Add(AddSpec{
		Kind:     KindStatus,
		Target:   "vyos",
		Subject:  "status.vyos",
		Callback: func(*nats.Msg) {},
	})
	if err != nil {
		t.Fatalf("expected nil add error, got %v", err)
	}

	sub := &nats.Subscription{}
	r.MarkActive(snap.ID, sub)

	registered, active := r.Counts()
	if registered != 1 {
		t.Fatalf("expected registered count %d, got %d", 1, registered)
	}
	if active != 1 {
		t.Fatalf("expected active count %d, got %d", 1, active)
	}

	handles := r.ClearActiveHandles()
	if len(handles) != 1 {
		t.Fatalf("expected %d active handle, got %d", 1, len(handles))
	}
	if handles[0].ID != snap.ID {
		t.Fatalf("expected handle id %q, got %q", snap.ID, handles[0].ID)
	}
	if handles[0].Sub != sub {
		t.Fatal("expected cleared handle to preserve subscription pointer")
	}

	_, active = r.Counts()
	if active != 0 {
		t.Fatalf("expected active count %d after clear, got %d", 0, active)
	}
}

/*
TC-REGISTRY-004
Type: Positive
Title: ClearActiveHandles is safe for repeated deactivation calls
Summary:
Verifies that deactivation can be called repeatedly without duplicate handles
or panics once active pointers have already been detached.

Validates:
  - first clear returns the active handle
  - second clear returns no handles
  - registry remains inactive after repeated clear calls
*/
func TestClearActiveHandlesIsIdempotent(t *testing.T) {
	r := New()
	snap, err := r.Add(AddSpec{
		Kind:     KindConfigure,
		Target:   "vyos",
		Subject:  "cmd.configure.vyos",
		Callback: func(*nats.Msg) {},
	})
	if err != nil {
		t.Fatalf("expected nil add error, got %v", err)
	}

	r.MarkActive(snap.ID, &nats.Subscription{})

	first := r.ClearActiveHandles()
	if len(first) != 1 {
		t.Fatalf("expected first clear to return %d handle, got %d", 1, len(first))
	}

	second := r.ClearActiveHandles()
	if len(second) != 0 {
		t.Fatalf("expected second clear to return %d handles, got %d", 0, len(second))
	}

	_, active := r.Counts()
	if active != 0 {
		t.Fatalf("expected active count %d after repeated clear, got %d", 0, active)
	}
}

/*
TC-REGISTRY-005
Type: Positive
Title: RestoreRecords returns saved registration intent snapshots
Summary:
Verifies that restore view returns activation records built from saved registry
intent so reconnect recovery can rehydrate subscriptions.

Validates:
  - restore records include all saved entries
  - restore record metadata preserves kind subject and queue-group values
*/
func TestRestoreRecordsReturnsSavedIntent(t *testing.T) {
	r := New()
	callback := func(*nats.Msg) {}

	if _, err := r.Add(AddSpec{
		Kind:       KindConfigure,
		Target:     "vyos",
		Subject:    "cmd.configure.vyos",
		QueueGroup: "cfg-workers",
		Callback:   callback,
	}); err != nil {
		t.Fatalf("expected nil add error, got %v", err)
	}
	if _, err := r.Add(AddSpec{
		Kind:     KindResult,
		Target:   "vyos",
		Subject:  "result.vyos",
		Callback: callback,
	}); err != nil {
		t.Fatalf("expected nil add error, got %v", err)
	}

	records := r.RestoreRecords()
	if len(records) != 2 {
		t.Fatalf("expected %d restore records, got %d", 2, len(records))
	}

	seen := map[string]ActivationRecord{}
	for _, rec := range records {
		seen[rec.Subject] = rec
	}
	if got, ok := seen["cmd.configure.vyos"]; !ok {
		t.Fatalf("expected restore record for subject %q", "cmd.configure.vyos")
	} else {
		if got.Kind != KindConfigure {
			t.Fatalf("expected configure kind %q, got %q", KindConfigure, got.Kind)
		}
		if got.QueueGroup != "cfg-workers" {
			t.Fatalf("expected queue group %q, got %q", "cfg-workers", got.QueueGroup)
		}
	}
	if got, ok := seen["result.vyos"]; !ok {
		t.Fatalf("expected restore record for subject %q", "result.vyos")
	} else if got.Kind != KindResult {
		t.Fatalf("expected result kind %q, got %q", KindResult, got.Kind)
	}
}

/*
TC-REGISTRY-006
Type: Positive
Title: Remove deletes intent key and detaches active handle
Summary:
Verifies that removing a registration deletes duplicate-detection state and
returns any detached active handle for caller-side cleanup.

Validates:
  - Remove returns detached active handle and true when entry exists
  - removed registration no longer contributes to counters
  - same registration can be added again after removal
*/
func TestRemoveDeletesIntentAndDetachesActiveHandle(t *testing.T) {
	r := New()
	snap, err := r.Add(AddSpec{
		Kind:     KindResult,
		Target:   "vyos",
		Subject:  "result.vyos",
		Callback: func(*nats.Msg) {},
	})
	if err != nil {
		t.Fatalf("expected nil add error, got %v", err)
	}

	activeSub := &nats.Subscription{}
	r.MarkActive(snap.ID, activeSub)

	handle, ok := r.Remove(snap.ID)
	if !ok {
		t.Fatal("expected remove to succeed")
	}
	if handle.ID != snap.ID {
		t.Fatalf("expected detached handle id %q, got %q", snap.ID, handle.ID)
	}
	if handle.Sub != activeSub {
		t.Fatal("expected detached active subscription handle to match")
	}

	registered, active := r.Counts()
	if registered != 0 {
		t.Fatalf("expected registered count %d after remove, got %d", 0, registered)
	}
	if active != 0 {
		t.Fatalf("expected active count %d after remove, got %d", 0, active)
	}

	if _, err := r.Add(AddSpec{
		Kind:     KindResult,
		Target:   "vyos",
		Subject:  "result.vyos",
		Callback: func(*nats.Msg) {},
	}); err != nil {
		t.Fatalf("expected re-add to succeed after remove, got %v", err)
	}
}
