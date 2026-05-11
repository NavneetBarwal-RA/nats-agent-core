//go:build integration
// +build integration

package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/routerarchitects/nats-agent-core/agentcore"
)

/*
TC-INTEGRATION-PHASE5-SEND-001
Type: Positive
Title: SubmitAction reaches registered action handler over public APIs
Summary:
Verifies that one started client can call SubmitAction while another started
client receives the action through RegisterActionHandler on real nats-server.

Validates:
  - SubmitAction returns accepted ack with expected subject and correlation fields
  - receiver action handler is invoked through public registration API
  - action payload and identity fields are preserved end-to-end
*/
func TestIntegrationSubmitActionReachesRegisteredActionHandler(t *testing.T) {
	srv := startTestNATSServer(t)
	bucket := uniqueName("cfg_desired")

	sender, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("sender New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, sender)

	receiver, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("receiver New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, receiver)

	received := make(chan agentcore.ActionCommand, 1)
	if err := receiver.RegisterActionHandler("vyos", "trace", func(_ context.Context, msg agentcore.ActionCommand) error {
		received <- msg
		return nil
	}); err != nil {
		t.Fatalf("RegisterActionHandler returned unexpected error: %v", err)
	}

	if err := receiver.Start(context.Background()); err != nil {
		t.Fatalf("receiver Start returned unexpected error: %v", err)
	}
	waitForActiveSubscriptions(t, receiver, 1)

	if err := sender.Start(context.Background()); err != nil {
		t.Fatalf("sender Start returned unexpected error: %v", err)
	}

	cmd := agentcore.ActionCommand{
		Version:     "1.0",
		RPCID:       "rpc-action-e2e-1",
		Target:      "vyos",
		CommandType: "action",
		Action:      "trace",
		Payload:     json.RawMessage(`{"destination":"8.8.8.8"}`),
		Timestamp:   time.Now().UTC(),
	}
	ack, err := sender.SubmitAction(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitAction returned unexpected error: %v", err)
	}
	if ack == nil {
		t.Fatal("expected non-nil submit action ack")
	}
	if !ack.Accepted {
		t.Fatal("expected submit action ack Accepted=true")
	}
	if ack.RPCID != cmd.RPCID {
		t.Fatalf("expected ack rpc_id %q, got %q", cmd.RPCID, ack.RPCID)
	}
	if ack.Target != cmd.Target {
		t.Fatalf("expected ack target %q, got %q", cmd.Target, ack.Target)
	}
	if ack.Subject != "cmd.action.vyos.trace" {
		t.Fatalf("expected ack subject %q, got %q", "cmd.action.vyos.trace", ack.Subject)
	}

	select {
	case got := <-received:
		if got.RPCID != cmd.RPCID {
			t.Fatalf("expected rpc_id %q, got %q", cmd.RPCID, got.RPCID)
		}
		if got.Target != cmd.Target {
			t.Fatalf("expected target %q, got %q", cmd.Target, got.Target)
		}
		if got.CommandType != cmd.CommandType {
			t.Fatalf("expected command_type %q, got %q", cmd.CommandType, got.CommandType)
		}
		if got.Action != cmd.Action {
			t.Fatalf("expected action %q, got %q", cmd.Action, got.Action)
		}
		if string(got.Payload) != string(cmd.Payload) {
			t.Fatalf("expected payload %s, got %s", string(cmd.Payload), string(got.Payload))
		}
		if got.Timestamp.IsZero() {
			t.Fatal("expected non-zero action timestamp")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for action handler delivery")
	}
}

/*
TC-INTEGRATION-PHASE5-SEND-002
Type: Positive
Title: PublishResult reaches registered result handler over public APIs
Summary:
Verifies that one started client can call PublishResult while another started
client receives the result through RegisterResultHandler on real nats-server.

Validates:
  - PublishResult succeeds through the public facade
  - receiver result handler is invoked through public registration API
  - result envelope identity and payload fields are preserved end-to-end
*/
func TestIntegrationPublishResultReachesRegisteredResultHandler(t *testing.T) {
	srv := startTestNATSServer(t)
	bucket := uniqueName("cfg_desired")

	controller, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("controller New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, controller)

	worker, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("worker New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, worker)

	received := make(chan agentcore.ResultEnvelope, 1)
	if err := controller.RegisterResultHandler("vyos", func(_ context.Context, msg agentcore.ResultEnvelope) error {
		received <- msg
		return nil
	}); err != nil {
		t.Fatalf("RegisterResultHandler returned unexpected error: %v", err)
	}

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("controller Start returned unexpected error: %v", err)
	}
	waitForActiveSubscriptions(t, controller, 1)

	if err := worker.Start(context.Background()); err != nil {
		t.Fatalf("worker Start returned unexpected error: %v", err)
	}

	msg := agentcore.ResultEnvelope{
		Version:     "1.0",
		RPCID:       "rpc-result-e2e-1",
		Target:      "vyos",
		CommandType: "action",
		Action:      "trace",
		Result:      "success",
		Payload:     json.RawMessage(`{"output":"ok"}`),
		Timestamp:   time.Now().UTC(),
	}
	if err := worker.PublishResult(context.Background(), msg); err != nil {
		t.Fatalf("PublishResult returned unexpected error: %v", err)
	}

	select {
	case got := <-received:
		if got.RPCID != msg.RPCID {
			t.Fatalf("expected rpc_id %q, got %q", msg.RPCID, got.RPCID)
		}
		if got.Target != msg.Target {
			t.Fatalf("expected target %q, got %q", msg.Target, got.Target)
		}
		if got.CommandType != msg.CommandType {
			t.Fatalf("expected command_type %q, got %q", msg.CommandType, got.CommandType)
		}
		if got.Action != msg.Action {
			t.Fatalf("expected action %q, got %q", msg.Action, got.Action)
		}
		if got.Result != msg.Result {
			t.Fatalf("expected result %q, got %q", msg.Result, got.Result)
		}
		if string(got.Payload) != string(msg.Payload) {
			t.Fatalf("expected payload %s, got %s", string(msg.Payload), string(got.Payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for result handler delivery")
	}
}

/*
TC-INTEGRATION-PHASE5-SEND-003
Type: Positive
Title: SubmitConfigure notifies target and desired config can be loaded
Summary:
Verifies the public configure store-then-notify flow between two clients and
proves the target can load desired config from KV after notification delivery.

Validates:
  - SubmitConfigure returns accepted ack with KV metadata
  - target configure handler receives configure notification over public API
  - target LoadDesiredConfig returns stored desired config matching submitted payload
*/
func TestIntegrationSubmitConfigureNotifiesReceiverAndDesiredConfigCanBeLoaded(t *testing.T) {
	srv := startTestNATSServer(t)
	bucket := uniqueName("cfg_desired")

	controller, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("controller New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, controller)

	target, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("target New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, target)

	notificationCh := make(chan agentcore.ConfigureNotification, 1)
	loadedCh := make(chan *agentcore.StoredDesiredConfig, 1)
	loadErrCh := make(chan error, 1)

	if err := target.RegisterConfigureHandler("vyos", func(ctx context.Context, msg agentcore.ConfigureNotification) error {
		notificationCh <- msg

		stored, err := target.LoadDesiredConfig(ctx, msg.Target)
		if err != nil {
			loadErrCh <- err
			return err
		}
		loadedCh <- stored
		return nil
	}); err != nil {
		t.Fatalf("RegisterConfigureHandler returned unexpected error: %v", err)
	}

	if err := target.Start(context.Background()); err != nil {
		t.Fatalf("target Start returned unexpected error: %v", err)
	}
	waitForActiveSubscriptions(t, target, 1)

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("controller Start returned unexpected error: %v", err)
	}

	cmd := agentcore.ConfigureCommand{
		Version:   "1.0",
		RPCID:     "rpc-config-e2e-1",
		Target:    "vyos",
		UUID:      "cfg-e2e-1",
		Payload:   json.RawMessage(`{"hostname":"router-1","interfaces":[]}`),
		Timestamp: time.Now().UTC(),
	}
	ack, err := controller.SubmitConfigure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitConfigure returned unexpected error: %v", err)
	}
	if ack == nil {
		t.Fatal("expected non-nil submit configure ack")
	}
	if !ack.Accepted {
		t.Fatal("expected submit configure ack Accepted=true")
	}
	if ack.RPCID != cmd.RPCID {
		t.Fatalf("expected ack rpc_id %q, got %q", cmd.RPCID, ack.RPCID)
	}
	if ack.Target != cmd.Target {
		t.Fatalf("expected ack target %q, got %q", cmd.Target, ack.Target)
	}
	if ack.Subject != "cmd.configure.vyos" {
		t.Fatalf("expected ack subject %q, got %q", "cmd.configure.vyos", ack.Subject)
	}
	if ack.KVBucket != bucket {
		t.Fatalf("expected ack KVBucket %q, got %q", bucket, ack.KVBucket)
	}
	if ack.KVKey != "desired.vyos" {
		t.Fatalf("expected ack KVKey %q, got %q", "desired.vyos", ack.KVKey)
	}
	if ack.KVRevision == 0 {
		t.Fatal("expected non-zero ack KVRevision")
	}

	select {
	case got := <-notificationCh:
		if got.RPCID != cmd.RPCID {
			t.Fatalf("expected notification rpc_id %q, got %q", cmd.RPCID, got.RPCID)
		}
		if got.Target != cmd.Target {
			t.Fatalf("expected notification target %q, got %q", cmd.Target, got.Target)
		}
		if got.UUID != cmd.UUID {
			t.Fatalf("expected notification uuid %q, got %q", cmd.UUID, got.UUID)
		}
		if got.CommandType != "configure" {
			t.Fatalf("expected notification command_type %q, got %q", "configure", got.CommandType)
		}
		if got.KVBucket != bucket {
			t.Fatalf("expected notification KVBucket %q, got %q", bucket, got.KVBucket)
		}
		if got.KVKey != "desired.vyos" {
			t.Fatalf("expected notification KVKey %q, got %q", "desired.vyos", got.KVKey)
		}
		if got.Timestamp.IsZero() {
			t.Fatal("expected non-zero notification timestamp")
		}
	case err := <-loadErrCh:
		t.Fatalf("LoadDesiredConfig returned unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for configure notification delivery")
	}

	select {
	case stored := <-loadedCh:
		if stored == nil {
			t.Fatal("expected non-nil stored desired config")
		}
		if stored.Bucket != bucket {
			t.Fatalf("expected loaded bucket %q, got %q", bucket, stored.Bucket)
		}
		if stored.Key != "desired.vyos" {
			t.Fatalf("expected loaded key %q, got %q", "desired.vyos", stored.Key)
		}
		if stored.Revision == 0 {
			t.Fatal("expected non-zero loaded revision")
		}
		if stored.Record.RPCID != cmd.RPCID {
			t.Fatalf("expected loaded record rpc_id %q, got %q", cmd.RPCID, stored.Record.RPCID)
		}
		if stored.Record.Target != cmd.Target {
			t.Fatalf("expected loaded record target %q, got %q", cmd.Target, stored.Record.Target)
		}
		if stored.Record.UUID != cmd.UUID {
			t.Fatalf("expected loaded record uuid %q, got %q", cmd.UUID, stored.Record.UUID)
		}
		if string(stored.Record.Payload) != string(cmd.Payload) {
			t.Fatalf("expected loaded payload %s, got %s", string(cmd.Payload), string(stored.Record.Payload))
		}
	case err := <-loadErrCh:
		t.Fatalf("LoadDesiredConfig returned unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for loaded desired config")
	}
}

/*
TC-INTEGRATION-PHASE5-SEND-004
Type: Positive
Title: PublishStatus reaches registered status handler over public APIs
Summary:
Verifies that one started client can call PublishStatus while another started
client receives the status through RegisterStatusHandler on real nats-server.

Validates:
  - PublishStatus succeeds through the public facade
  - observer status handler is invoked through public registration API
  - status envelope fields are preserved end-to-end
*/
func TestIntegrationPublishStatusReachesRegisteredStatusHandler(t *testing.T) {
	srv := startTestNATSServer(t)
	bucket := uniqueName("cfg_desired")

	observer, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("observer New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, observer)

	target, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("target New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, target)

	received := make(chan agentcore.StatusEnvelope, 1)
	if err := observer.RegisterStatusHandler("vyos", func(_ context.Context, msg agentcore.StatusEnvelope) error {
		received <- msg
		return nil
	}); err != nil {
		t.Fatalf("RegisterStatusHandler returned unexpected error: %v", err)
	}

	if err := observer.Start(context.Background()); err != nil {
		t.Fatalf("observer Start returned unexpected error: %v", err)
	}
	waitForActiveSubscriptions(t, observer, 1)

	if err := target.Start(context.Background()); err != nil {
		t.Fatalf("target Start returned unexpected error: %v", err)
	}

	msg := agentcore.StatusEnvelope{
		Version:   "1.0",
		RPCID:     "rpc-status-e2e-1",
		Target:    "vyos",
		Status:    "running",
		Stage:     "startup",
		Payload:   json.RawMessage(`{"ready":true}`),
		Timestamp: time.Now().UTC(),
	}
	if err := target.PublishStatus(context.Background(), msg); err != nil {
		t.Fatalf("PublishStatus returned unexpected error: %v", err)
	}

	select {
	case got := <-received:
		if got.RPCID != msg.RPCID {
			t.Fatalf("expected rpc_id %q, got %q", msg.RPCID, got.RPCID)
		}
		if got.Target != msg.Target {
			t.Fatalf("expected target %q, got %q", msg.Target, got.Target)
		}
		if got.Status != msg.Status {
			t.Fatalf("expected status %q, got %q", msg.Status, got.Status)
		}
		if got.Stage != msg.Stage {
			t.Fatalf("expected stage %q, got %q", msg.Stage, got.Stage)
		}
		if string(got.Payload) != string(msg.Payload) {
			t.Fatalf("expected payload %s, got %s", string(msg.Payload), string(got.Payload))
		}
		if got.Timestamp.IsZero() {
			t.Fatal("expected non-zero status timestamp")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for status handler delivery")
	}
}

/*
TC-INTEGRATION-PHASE5-SEND-005
Type: Positive
Title: Public action/result round trip preserves rpc_id
Summary:
Verifies a complete public controller-to-worker action flow where the worker
receives an action and publishes a result with the same rpc_id, which the
controller receives through its registered result handler.

Validates:
  - controller SubmitAction reaches worker RegisterActionHandler
  - worker PublishResult reaches controller RegisterResultHandler
  - rpc_id, target, action, result, and payload are preserved end-to-end
*/
func TestIntegrationPublicActionResultRoundTrip(t *testing.T) {
	srv := startTestNATSServer(t)
	bucket := uniqueName("cfg_desired")

	controller, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("controller New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, controller)

	worker, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))
	if err != nil {
		t.Fatalf("worker New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, worker)

	resultCh := make(chan agentcore.ResultEnvelope, 1)
	resultPublishErrCh := make(chan error, 1)
	if err := controller.RegisterResultHandler("vyos", func(_ context.Context, msg agentcore.ResultEnvelope) error {
		resultCh <- msg
		return nil
	}); err != nil {
		t.Fatalf("controller RegisterResultHandler returned unexpected error: %v", err)
	}

	actionCh := make(chan agentcore.ActionCommand, 1)
	if err := worker.RegisterActionHandler("vyos", "trace", func(ctx context.Context, msg agentcore.ActionCommand) error {
		actionCh <- msg

		err := worker.PublishResult(ctx, agentcore.ResultEnvelope{
			Version:     msg.Version,
			RPCID:       msg.RPCID,
			Target:      msg.Target,
			CommandType: msg.CommandType,
			Action:      msg.Action,
			Result:      "success",
			Payload:     json.RawMessage(`{"output":"trace ok"}`),
			Timestamp:   time.Now().UTC(),
		})
		if err != nil {
			resultPublishErrCh <- err
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("worker RegisterActionHandler returned unexpected error: %v", err)
	}

	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("controller Start returned unexpected error: %v", err)
	}
	waitForActiveSubscriptions(t, controller, 1)

	if err := worker.Start(context.Background()); err != nil {
		t.Fatalf("worker Start returned unexpected error: %v", err)
	}
	waitForActiveSubscriptions(t, worker, 1)

	cmd := agentcore.ActionCommand{
		Version:     "1.0",
		RPCID:       "rpc-action-roundtrip-1",
		Target:      "vyos",
		CommandType: "action",
		Action:      "trace",
		Payload:     json.RawMessage(`{"destination":"8.8.8.8"}`),
		Timestamp:   time.Now().UTC(),
	}
	ack, err := controller.SubmitAction(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitAction returned unexpected error: %v", err)
	}
	if ack == nil {
		t.Fatal("expected non-nil SubmitAction ack")
	}
	if !ack.Accepted {
		t.Fatal("expected SubmitAction ack Accepted=true")
	}
	if ack.RPCID != cmd.RPCID {
		t.Fatalf("expected ack rpc_id %q, got %q", cmd.RPCID, ack.RPCID)
	}
	if ack.Target != "vyos" {
		t.Fatalf("expected ack target %q, got %q", "vyos", ack.Target)
	}
	if ack.Subject != "cmd.action.vyos.trace" {
		t.Fatalf("expected ack subject %q, got %q", "cmd.action.vyos.trace", ack.Subject)
	}

	var gotAction agentcore.ActionCommand
	select {
	case gotAction = <-actionCh:
	case err := <-resultPublishErrCh:
		t.Fatalf("worker PublishResult returned unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf(
			"timed out waiting for worker action handler delivery (controller health: state=%s active_subs=%d, worker health: state=%s active_subs=%d)",
			controller.Health().State,
			controller.Health().ActiveSubscriptions,
			worker.Health().State,
			worker.Health().ActiveSubscriptions,
		)
	}
	if gotAction.RPCID != cmd.RPCID {
		t.Fatalf("expected action rpc_id %q, got %q", cmd.RPCID, gotAction.RPCID)
	}
	if gotAction.Target != cmd.Target {
		t.Fatalf("expected action target %q, got %q", cmd.Target, gotAction.Target)
	}
	if gotAction.Action != cmd.Action {
		t.Fatalf("expected action name %q, got %q", cmd.Action, gotAction.Action)
	}
	if string(gotAction.Payload) != string(cmd.Payload) {
		t.Fatalf("expected action payload %s, got %s", string(cmd.Payload), string(gotAction.Payload))
	}

	select {
	case got := <-resultCh:
		if got.RPCID != cmd.RPCID {
			t.Fatalf("expected result rpc_id %q, got %q", cmd.RPCID, got.RPCID)
		}
		if got.Target != "vyos" {
			t.Fatalf("expected result target %q, got %q", "vyos", got.Target)
		}
		if got.CommandType != "action" {
			t.Fatalf("expected result command_type %q, got %q", "action", got.CommandType)
		}
		if got.Action != "trace" {
			t.Fatalf("expected result action %q, got %q", "trace", got.Action)
		}
		if got.Result != "success" {
			t.Fatalf("expected result status %q, got %q", "success", got.Result)
		}
		if string(got.Payload) != `{"output":"trace ok"}` {
			t.Fatalf("expected result payload %s, got %s", `{"output":"trace ok"}`, string(got.Payload))
		}
		if got.Timestamp.IsZero() {
			t.Fatal("expected non-zero result timestamp")
		}
	case err := <-resultPublishErrCh:
		t.Fatalf("worker PublishResult returned unexpected error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for controller result handler delivery")
	}
}

/*
TC-INTEGRATION-PHASE5-SEND-006
Type: Positive
Title: Reconnect restore receives public publish after server restart
Summary:
Verifies that a restored result subscription receives messages published through
the public PublishResult API after a real nats-server restart, without handler
re-registration.

Validates:
  - result delivery works before restart through public PublishResult
  - receiver reconnects and restores active subscription intent
  - public PublishResult after restart is delivered to restored handler
*/
func TestIntegrationReconnectRestoreReceivesPublicPublishAfterServerRestart(t *testing.T) {
	srv := startTestNATSServer(t)
	bucket := uniqueName("cfg_desired")

	receiverCfg := newIntegrationConfig(srv.URL, bucket, true)
	receiverCfg.NATS.MaxReconnects = 50
	receiverCfg.NATS.ReconnectWait = 100 * time.Millisecond

	publisherCfg := newIntegrationConfig(srv.URL, bucket, true)
	publisherCfg.NATS.MaxReconnects = 50
	publisherCfg.NATS.ReconnectWait = 100 * time.Millisecond

	receiver, err := agentcore.New(receiverCfg)
	if err != nil {
		t.Fatalf("receiver New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, receiver)

	publisher, err := agentcore.New(publisherCfg)
	if err != nil {
		t.Fatalf("publisher New returned unexpected error: %v", err)
	}
	defer closeIntegrationClient(t, publisher)

	received := make(chan string, 4)
	if err := receiver.RegisterResultHandler("vyos", func(_ context.Context, msg agentcore.ResultEnvelope) error {
		received <- msg.RPCID
		return nil
	}); err != nil {
		t.Fatalf("receiver RegisterResultHandler returned unexpected error: %v", err)
	}

	if err := receiver.Start(context.Background()); err != nil {
		t.Fatalf("receiver Start returned unexpected error: %v", err)
	}
	waitForActiveSubscriptions(t, receiver, 1)

	if err := publisher.Start(context.Background()); err != nil {
		t.Fatalf("publisher Start returned unexpected error: %v", err)
	}

	before := agentcore.ResultEnvelope{
		Version:   "1.0",
		RPCID:     "rpc-public-before-restart",
		Target:    "vyos",
		Result:    "ok",
		Timestamp: time.Now().UTC(),
	}
	if err := publisher.PublishResult(context.Background(), before); err != nil {
		t.Fatalf("PublishResult before restart returned unexpected error: %v", err)
	}

	select {
	case got := <-received:
		if got != before.RPCID {
			t.Fatalf("expected pre-restart rpc_id %q, got %q", before.RPCID, got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for pre-restart result delivery")
	}

	srv.restart(t)

	if err := waitForClientConnected(receiver, 10*time.Second); err != nil {
		t.Fatalf("receiver did not reconnect with active subscriptions: %v", err)
	}
	waitForClientConnectedState(t, publisher, 10*time.Second)

	after := agentcore.ResultEnvelope{
		Version:   "1.0",
		RPCID:     "rpc-public-after-restart",
		Target:    "vyos",
		Result:    "ok",
		Timestamp: time.Now().UTC(),
	}
	if err := publisher.PublishResult(context.Background(), after); err != nil {
		t.Fatalf("PublishResult after restart returned unexpected error: %v", err)
	}

	select {
	case got := <-received:
		if got != after.RPCID {
			t.Fatalf("expected post-restart rpc_id %q, got %q", after.RPCID, got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for post-restart result delivery")
	}
}

func waitForClientConnectedState(t *testing.T, client *agentcore.Client, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if client.Health().State == agentcore.StateConnected {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for client connected state, got %s", client.Health().State)
}

func waitForActiveSubscriptions(t *testing.T, client *agentcore.Client, want int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if client.Health().ActiveSubscriptions >= want {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d active subscriptions, got %d", want, client.Health().ActiveSubscriptions)
}

func closeIntegrationClient(t *testing.T, client *agentcore.Client) {
	t.Helper()
	if client == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Close(ctx); err != nil {
		t.Logf("client close returned error: %v", err)
	}
}
