Read the current codebase carefully before making changes.

This is a Phase 5 integration-test hardening prompt.

The main public sender-to-receiver integration tests are already implemented:

- SubmitAction(...) -> RegisterActionHandler(...)
- PublishResult(...) -> RegisterResultHandler(...)
- SubmitConfigure(...) -> RegisterConfigureHandler(...)
- receiver LoadDesiredConfig(...) after configure notification
- PublishStatus(...) -> RegisterStatusHandler(...)

Now add two additional useful hardening integration tests:

1. full public action/result round trip
2. reconnect restore verified with public sender API after reconnect

Do not change public API signatures.
Do not change production behavior unless a test exposes a real bug.
Do not add workload/business logic.
Do not add Phase 6 observability changes.
Do not update README or PR description in this prompt.

Use real nats-server.
Use public agentcore APIs wherever possible.
Use PROMPTS/TESTS_STANDARD_TEMPLATE.md block comment format.

Preferred target file:

    tests/integration/phase5_public_send_receive_integration_test.go

Use the next sequential TC IDs after existing public send/receive integration tests:

- TC-INTEGRATION-PHASE5-SEND-005
- TC-INTEGRATION-PHASE5-SEND-006

---

## Test 1: full public action/result round trip

Suggested test name:

    TestIntegrationPublicActionResultRoundTrip

Goal:

Verify this complete public API flow:

    controller RegisterResultHandler(...)
      worker RegisterActionHandler(...)
      controller SubmitAction(...)
        -> real NATS
      worker action handler receives ActionCommand
      worker PublishResult(...) with same rpc_id
        -> real NATS
      controller result handler receives ResultEnvelope

This test proves a real controller/worker action-result workflow using public APIs only.

Steps:

1. Start real nats-server:

       srv := startTestNATSServer(t)

2. Create unique bucket:

       bucket := uniqueName("cfg_desired")

3. Create controller client:

       controller, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))

4. Create worker client:

       worker, err := agentcore.New(newIntegrationConfig(srv.URL, bucket, true))

5. Register result handler on controller before Start:

       resultCh := make(chan agentcore.ResultEnvelope, 1)

       controller.RegisterResultHandler("vyos", func(ctx context.Context, msg agentcore.ResultEnvelope) error {
           resultCh <- msg
           return nil
       })

6. Register action handler on worker before Start:

       actionCh := make(chan agentcore.ActionCommand, 1)

       worker.RegisterActionHandler("vyos", "trace", func(ctx context.Context, msg agentcore.ActionCommand) error {
           actionCh <- msg

           return worker.PublishResult(ctx, agentcore.ResultEnvelope{
               Version:     msg.Version,
               RPCID:       msg.RPCID,
               Target:      msg.Target,
               CommandType: msg.CommandType,
               Action:      msg.Action,
               Result:      "success",
               Payload:     json.RawMessage(`{"output":"trace ok"}`),
               Timestamp:   time.Now().UTC(),
           })
       })

7. Start controller, wait for one active subscription.

8. Start worker, wait for one active subscription.

9. Controller submits action:

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

10. Assert SubmitAction ack:
    - ack is non-nil
    - Accepted == true
    - RPCID == cmd.RPCID
    - Target == "vyos"
    - Subject == "cmd.action.vyos.trace"

11. Assert worker action handler received the command:
    - RPCID preserved
    - Target preserved
    - Action preserved
    - Payload preserved

12. Assert controller result handler received result:
    - RPCID == cmd.RPCID
    - Target == "vyos"
    - CommandType == "action"
    - Action == "trace"
    - Result == "success"
    - Payload == `{"output":"trace ok"}`
    - Timestamp is not zero

Important:
- Do not use raw NATS publish in this test.
- The worker action handler should publish the result using public worker.PublishResult(...).
- Use buffered channels.
- Use select with 5 second timeout.

TC block:

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

---

## Test 2: reconnect restore with public sender after reconnect

Suggested test name:

    TestIntegrationReconnectRestoreReceivesPublicPublishAfterServerRestart

Goal:

Existing reconnect restore test uses raw NATS publish after reconnect.
Add a hardening test that verifies restored subscriptions also receive messages
sent through the public PublishResult(...) API after a server restart.

Flow:

    receiver RegisterResultHandler(...)
    receiver Start(...)
    publisher Start(...)
    publisher PublishResult("before restart")
      -> receiver gets before message
    restart nats-server
    wait for receiver reconnect
    wait for publisher reconnect
    publisher PublishResult("after restart")
      -> receiver gets after message without re-registering handler

Steps:

1. Start real nats-server.

2. Create unique bucket.

3. Create receiver config:
   - use newIntegrationConfig(srv.URL, bucket, true)
   - set MaxReconnects high enough, e.g. 50
   - set ReconnectWait to 100ms

4. Create publisher config with same reconnect settings.

5. Create receiver client.

6. Create publisher client.

7. Register result handler on receiver before Start:

       received := make(chan string, 4)

       receiver.RegisterResultHandler("vyos", func(ctx context.Context, msg agentcore.ResultEnvelope) error {
           received <- msg.RPCID
           return nil
       })

8. Start receiver and wait for active subscription.

9. Start publisher.

10. Publish first result using public API:

       publisher.PublishResult(context.Background(), agentcore.ResultEnvelope{
           Version:   "1.0",
           RPCID:     "rpc-public-before-restart",
           Target:    "vyos",
           Result:    "ok",
           Timestamp: time.Now().UTC(),
       })

11. Assert receiver gets `"rpc-public-before-restart"`.

12. Restart server:

       srv.restart(t)

13. Wait for receiver connected and active subscription.
    Existing helper waitForClientConnected(client, timeout) may require ActiveSubscriptions >= 1.
    Use it if available.

14. Wait for publisher connected.
    Existing waitForClientConnected may require ActiveSubscriptions >= 1, which publisher will not have.
    If needed, add a new helper:

       func waitForClientState(t *testing.T, client *agentcore.Client, want agentcore.ConnectionState, timeout time.Duration)

    or:

       func waitForClientConnectedState(t *testing.T, client *agentcore.Client, timeout time.Duration)

    It should only require:

       client.Health().State == agentcore.StateConnected

    Do not require ActiveSubscriptions for publisher.

15. Publish second result through public API:

       publisher.PublishResult(context.Background(), agentcore.ResultEnvelope{
           Version:   "1.0",
           RPCID:     "rpc-public-after-restart",
           Target:    "vyos",
           Result:    "ok",
           Timestamp: time.Now().UTC(),
       })

16. Assert receiver gets `"rpc-public-after-restart"` without re-registering handler.

Important:
- Do not use raw NATS publish in this test.
- The receiver must not re-register its handler after restart.
- The after-restart message must be sent using public publisher.PublishResult(...).
- Use timeouts and reconnect wait helpers.
- Avoid arbitrary long sleeps. Poll health with a short interval.

TC block:

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

---

## Helper guidance

Reuse existing helper if available:

    waitForActiveSubscriptions(t, client, want)
    closeIntegrationClient(t, client)
    waitForClientConnected(client, timeout)

If adding a new helper for publisher reconnect, keep it local to the integration test file:

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

Do not duplicate helper names already defined in the same package.

---

## Duplicate test guidance

These two tests are not duplicates of the existing tests.

Existing tests cover:
- individual public send to individual receive handler
- raw NATS receive-side behavior
- raw NATS reconnect restore

New tests should cover:
- chained public action/result round trip in one workflow
- reconnect restore with public PublishResult after reconnect

Do not rewrite existing tests.
Do not delete existing tests.
Do not duplicate existing test names or TC IDs.

---

## Commands to run

Run:

    gofmt on changed Go files
    go test ./...
    go test -race ./...

Run integration tests:

    go test -count=1 -v -tags=integration ./tests/integration/...
    go test -count=1 -race -tags=integration ./tests/integration/...

If nats-server is not installed and integration tests are skipped, say clearly that they were skipped.
Do not claim integration tests passed unless they actually ran.

---

## Final response required from Codex

After coding, summarize:

1. Files changed
2. New integration tests added with TC IDs
3. What action/result round trip verifies
4. What reconnect/public-publish test verifies
5. Whether raw NATS publishing was avoided in the new tests
6. Whether production code changed
7. Commands run and results
8. Any skipped tests and why
9. Confirmation that public APIs did not change
10. Confirmation that no workload/business logic was added
11. Confirmation that Phase 6 observability was not added
