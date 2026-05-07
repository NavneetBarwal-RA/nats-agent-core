//go:build integration
// +build integration

package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/routerarchitects/nats-agent-core/agentcore"
)

/*
TC-INT-PHASE5-001
Type: Positive
Title: Result handler receives real NATS result messages after start
Summary:
Verifies Phase 5 result subscription wiring end-to-end with a real nats-server:
pre-start registration is activated on Start and receives published result data.

Validates:
  - RegisterResultHandler before Start succeeds
  - real publish to result.<target> is delivered to handler
  - receive-side rpc_id is preserved exactly
*/
func TestIntegrationResultHandlerReceivesRealPublishedResult(t *testing.T) {
	srv := startTestNATSServer(t)
	bucket := uniqueName("cfg_desired")

	client, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("New returned unexpected error: %v", err)
	}
	defer func() {
		_ = client.Close(context.Background())
	}()

	received := make(chan agentcore.ResultEnvelope, 1)
	if err := client.RegisterResultHandler("vyos", func(_ context.Context, msg agentcore.ResultEnvelope) error {
		received <- msg
		return nil
	}); err != nil {
		t.Fatalf("RegisterResultHandler returned unexpected error: %v", err)
	}

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start returned unexpected error: %v", err)
	}

	payload := agentcore.ResultEnvelope{
		Version:   "1.0",
		RPCID:     "rpc-res-live-1",
		Target:    "vyos",
		Result:    "ok",
		Timestamp: time.Now().UTC(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to encode result payload: %v", err)
	}

	pub, err := nats.Connect(srv.URL, nats.NoReconnect())
	if err != nil {
		t.Fatalf("failed to connect publisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Publish("result.vyos", raw); err != nil {
		t.Fatalf("Publish returned unexpected error: %v", err)
	}
	if err := pub.Flush(); err != nil {
		t.Fatalf("Flush returned unexpected error: %v", err)
	}

	select {
	case got := <-received:
		if got.RPCID != payload.RPCID {
			t.Fatalf("expected rpc_id %q, got %q", payload.RPCID, got.RPCID)
		}
		if got.Target != payload.Target || got.Result != payload.Result {
			t.Fatalf("unexpected result payload: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for result handler callback")
	}
}

/*
TC-INT-PHASE5-002
Type: Positive
Title: Action handler receives real NATS action command messages after start
Summary:
Verifies Phase 5 action subscription wiring end-to-end with a real nats-server.

Validates:
  - RegisterActionHandler before Start succeeds
  - real publish to cmd.action.<target>.<action> is delivered to handler
  - handler receives target/action/rpc_id/payload values
*/
func TestIntegrationActionHandlerReceivesRealPublishedAction(t *testing.T) {
	srv := startTestNATSServer(t)
	bucket := uniqueName("cfg_desired")

	client, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("New returned unexpected error: %v", err)
	}
	defer func() {
		_ = client.Close(context.Background())
	}()

	received := make(chan agentcore.ActionCommand, 1)
	if err := client.RegisterActionHandler("vyos", "trace", func(_ context.Context, msg agentcore.ActionCommand) error {
		received <- msg
		return nil
	}); err != nil {
		t.Fatalf("RegisterActionHandler returned unexpected error: %v", err)
	}

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start returned unexpected error: %v", err)
	}

	payload := agentcore.ActionCommand{
		Version:     "1.0",
		RPCID:       "rpc-act-live-1",
		Target:      "vyos",
		CommandType: "action",
		Action:      "trace",
		Payload:     json.RawMessage(`{"destination":"8.8.8.8"}`),
		Timestamp:   time.Now().UTC(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to encode action payload: %v", err)
	}

	pub, err := nats.Connect(srv.URL, nats.NoReconnect())
	if err != nil {
		t.Fatalf("failed to connect publisher: %v", err)
	}
	defer pub.Close()

	if err := pub.Publish("cmd.action.vyos.trace", raw); err != nil {
		t.Fatalf("Publish returned unexpected error: %v", err)
	}
	if err := pub.Flush(); err != nil {
		t.Fatalf("Flush returned unexpected error: %v", err)
	}

	select {
	case got := <-received:
		if got.RPCID != payload.RPCID {
			t.Fatalf("expected rpc_id %q, got %q", payload.RPCID, got.RPCID)
		}
		if got.Target != payload.Target || got.Action != payload.Action {
			t.Fatalf("unexpected action identity: %+v", got)
		}
		if string(got.Payload) != string(payload.Payload) {
			t.Fatalf("expected payload %s, got %s", string(payload.Payload), string(got.Payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for action handler callback")
	}
}

/*
TC-INT-PHASE5-003
Type: Positive
Title: Result handler continues after reconnect restore without re-register
Summary:
Verifies that after a real server restart/reconnect cycle, registered result
handler intent is restored and subsequent messages are still delivered.

Validates:
  - message is received before server restart
  - client reconnects after restart
  - message is received after reconnect without re-registering handlers
*/
func TestIntegrationReconnectRestoreDeliversAfterServerRestart(t *testing.T) {
	srv := startTestNATSServer(t)
	bucket := uniqueName("cfg_desired")

	cfg := newIntegrationConfig(srv.URL, bucket, true)
	cfg.NATS.MaxReconnects = 50
	cfg.NATS.ReconnectWait = 100 * time.Millisecond

	client, err := agentcore.New(cfg)
	if err != nil {
		t.Fatalf("New returned unexpected error: %v", err)
	}
	defer func() {
		_ = client.Close(context.Background())
	}()

	received := make(chan string, 4)
	if err := client.RegisterResultHandler("vyos", func(_ context.Context, msg agentcore.ResultEnvelope) error {
		received <- msg.RPCID
		return nil
	}); err != nil {
		t.Fatalf("RegisterResultHandler returned unexpected error: %v", err)
	}

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start returned unexpected error: %v", err)
	}

	publishResult := func(rpcID string) {
		t.Helper()
		payload := agentcore.ResultEnvelope{
			Version:   "1.0",
			RPCID:     rpcID,
			Target:    "vyos",
			Result:    "ok",
			Timestamp: time.Now().UTC(),
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("failed to encode result payload: %v", err)
		}
		pub, err := nats.Connect(srv.URL, nats.NoReconnect())
		if err != nil {
			t.Fatalf("failed to connect publisher: %v", err)
		}
		defer pub.Close()
		if err := pub.Publish("result.vyos", raw); err != nil {
			t.Fatalf("Publish returned unexpected error: %v", err)
		}
		if err := pub.Flush(); err != nil {
			t.Fatalf("Flush returned unexpected error: %v", err)
		}
	}

	publishResult("rpc-before-restart")
	select {
	case got := <-received:
		if got != "rpc-before-restart" {
			t.Fatalf("expected rpc_id %q, got %q", "rpc-before-restart", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for pre-restart result delivery")
	}

	srv.restart(t)

	if err := waitForClientConnected(client, 10*time.Second); err != nil {
		t.Fatalf("client did not reconnect after server restart: %v", err)
	}

	publishResult("rpc-after-restart")
	select {
	case got := <-received:
		if got != "rpc-after-restart" {
			t.Fatalf("expected rpc_id %q, got %q", "rpc-after-restart", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for post-reconnect result delivery")
	}
}

func waitForClientConnected(client *agentcore.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		health := client.Health()
		if health.State == agentcore.StateConnected && health.ActiveSubscriptions >= 1 {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return context.DeadlineExceeded
}
