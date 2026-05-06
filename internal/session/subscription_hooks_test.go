package session

import "testing"

/*
TC-SESSION-HOOKS-001
Type: Positive
Title: SetSubscriptionCounts updates health subscription counters
Summary:
Verifies that session health snapshot reflects subscription registry counters
when subscription lifecycle updates are applied by the client facade.

Validates:
  - registered subscription counter is updated
  - active subscription counter is updated
*/
func TestSetSubscriptionCountsUpdatesHealthSnapshot(t *testing.T) {
	m, err := NewManager(testSessionConfig(), Hooks{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	m.SetSubscriptionCounts(7, 3)
	got := m.HealthSnapshot()

	if got.RegisteredSubscriptions != 7 {
		t.Fatalf("expected RegisteredSubscriptions %d, got %d", 7, got.RegisteredSubscriptions)
	}
	if got.ActiveSubscriptions != 3 {
		t.Fatalf("expected ActiveSubscriptions %d, got %d", 3, got.ActiveSubscriptions)
	}
}

/*
TC-SESSION-HOOKS-002
Type: Positive
Title: SetReconnectHandler stores reconnect callback hook
Summary:
Verifies that reconnect callback setter updates manager hook storage so
reconnect restore wiring can be invoked after session rebound.

Validates:
  - reconnect hook setter stores the callback
  - stored callback can be invoked safely
*/
func TestSetReconnectHandlerStoresCallback(t *testing.T) {
	m, err := NewManager(testSessionConfig(), Hooks{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	calls := 0
	m.SetReconnectHandler(func() {
		calls++
	})

	if m.hooks.OnReconnected == nil {
		t.Fatal("expected reconnect hook to be set")
	}

	m.hooks.OnReconnected()
	if calls != 1 {
		t.Fatalf("expected reconnect callback calls %d, got %d", 1, calls)
	}
}
