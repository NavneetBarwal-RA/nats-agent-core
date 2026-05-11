Read the current codebase carefully before making changes.

This is a Phase 5 integration-test prompt.

The public send APIs are now implemented and unit-tested:

- SubmitConfigure(...)
- SubmitAction(...)
- PublishResult(...)
- PublishStatus(...)

The receive APIs are also implemented and integration-tested:

- RegisterConfigureHandler(...)
- RegisterActionHandler(...)
- RegisterResultHandler(...)
- RegisterStatusHandler(...)

Now add true public sender-to-receiver integration tests using real nats-server and public library APIs on both sides.

Do not add unit tests in this prompt.
Do not change public API signatures.
Do not change production behavior unless an integration test exposes a real bug.
Do not use raw NATS publishing for the main test flows.
Do not add workload/business logic.
Do not add Phase 6 observability changes.
Do not update README or PR description in this prompt unless needed for test instructions.

Use the existing integration test infrastructure and style.

Relevant files:

- tests/integration/phase5_subscribe_reconnect_integration_test.go
- tests/integration/testserver_test.go
- agentcore/client.go
- agentcore/client_send_test.go
- PROMPTS/TESTS_STANDARD_TEMPLATE.md

Existing integration helpers to reuse:

- startTestNATSServer(t)
- newIntegrationConfig(...)
- uniqueName(...)
- any existing publishJSON/publishRaw helpers only for old raw-NATS tests, not for these new public sender-to-receiver flows

These new tests must use real nats-server and two real agentcore.Client instances where appropriate.

---

## Required integration scenarios

Add these true public sender-to-receiver integration tests:

1. client A SubmitAction(...) → client B RegisterActionHandler(...)
2. client B PublishResult(...) → client A RegisterResultHandler(...)
3. client A SubmitConfigure(...) → client B RegisterConfigureHandler(...)
4. client B loads desired config from KV after configure notification
5. client B PublishStatus(...) → client A RegisterStatusHandler(...)

Prefer adding them to:

    tests/integration/phase5_subscribe_reconnect_integration_test.go

If the file becomes too large, it is acceptable to create:

    tests/integration/phase5_public_send_receive_integration_test.go

Use the same integration build tags as existing integration tests:

    //go:build integration
    // +build integration

Every new test function must use the repository TC block comment format from:

    PROMPTS/TESTS_STANDARD_TEMPLATE.md

Use sequential IDs such as:

- TC-INTEGRATION-PHASE5-SEND-001
- TC-INTEGRATION-PHASE5-SEND-002
- TC-INTEGRATION-PHASE5-SEND-003

Use clear Type, Title, Summary, and Validates sections.

---

## Important design requirement

These tests must prove public library-to-library flow.

That means:

- sender must call public send API:
  - SubmitAction(...)
  - SubmitConfigure(...)
  - PublishResult(...)
  - PublishStatus(...)

- receiver must use public receive API:
  - RegisterActionHandler(...)
  - RegisterConfigureHandler(...)
  - RegisterResultHandler(...)
  - RegisterStatusHandler(...)

Do not inject the main message using raw nats.Conn.Publish(...).

Raw NATS may only be used by existing tests or for setup if absolutely necessary. The new tests should prove the public facade works end-to-end.

---

## Test 1: SubmitAction reaches registered action handler

Suggested name:

    TestIntegrationSubmitActionReachesRegisteredActionHandler

Flow:

1. Start real nats-server with startTestNATSServer(t).
2. Create unique bucket name.
3. Create sender client A using newIntegrationConfig(serverURL, bucket, true).
4. Create receiver client B using newIntegrationConfig(serverURL, bucket, true).
5. Register action handler on receiver before Start(ctx):

       receiver.RegisterActionHandler("vyos", "trace", func(ctx context.Context, msg agentcore.ActionCommand) error {
           received <- msg
           return nil
       })

6. Start receiver.
7. Start sender.
8. Call sender.SubmitAction(ctx, agentcore.ActionCommand{...}).

Payload example:

    {
      "destination": "8.8.8.8"
    }

Use fields:
- Version: "1.0"
- RPCID: "rpc-action-e2e-1"
- Target: "vyos"
- CommandType: "action"
- Action: "trace"
- Payload: json.RawMessage(`{"destination":"8.8.8.8"}`)
- Timestamp: time.Now().UTC()

9. Assert ack:
- non-nil
- Accepted == true
- RPCID preserved
- Target == "vyos"
- Subject == "cmd.action.vyos.trace"

10. Wait for receiver channel with timeout.
11. Assert received action:
- RPCID preserved
- Target preserved
- CommandType preserved
- Action preserved
- Payload preserved
- Timestamp is not zero

TC metadata:
- ID: TC-INTEGRATION-PHASE5-SEND-001
- Type: Positive

This test proves public SubmitAction publishes to real NATS and public RegisterActionHandler receives it.

---

## Test 2: PublishResult reaches registered result handler

Suggested name:

    TestIntegrationPublishResultReachesRegisteredResultHandler

Flow:

1. Start real nats-server.
2. Create unique bucket.
3. Create controller client A.
4. Create worker client B.
5. Register result handler on controller before Start(ctx):

       controller.RegisterResultHandler("vyos", func(ctx context.Context, msg agentcore.ResultEnvelope) error {
           received <- msg
           return nil
       })

6. Start controller.
7. Start worker.
8. Call worker.PublishResult(ctx, agentcore.ResultEnvelope{...}).

Use fields:
- Version: "1.0"
- RPCID: "rpc-result-e2e-1"
- Target: "vyos"
- CommandType: "action"
- Action: "trace"
- Result: "success"
- Payload: json.RawMessage(`{"output":"ok"}`)
- Timestamp: time.Now().UTC()

9. Wait for controller result handler.
10. Assert:
- RPCID preserved
- Target preserved
- CommandType preserved
- Action preserved
- Result == "success"
- Payload preserved

TC metadata:
- ID: TC-INTEGRATION-PHASE5-SEND-002
- Type: Positive

This test proves public PublishResult and public RegisterResultHandler work together over real NATS.

---

## Test 3: SubmitConfigure reaches configure handler and receiver loads desired config from KV

Suggested name:

    TestIntegrationSubmitConfigureNotifiesReceiverAndDesiredConfigCanBeLoaded

This is the most important configure end-to-end test.

Flow:

1. Start real nats-server.
2. Create unique bucket.
3. Create controller client A using newIntegrationConfig(serverURL, bucket, true).
4. Create target client B using newIntegrationConfig(serverURL, bucket, true).

Important:
Both clients should use the same KV bucket so the target can load desired config stored by the controller.

5. Register configure handler on target before Start(ctx):

       target.RegisterConfigureHandler("vyos", func(ctx context.Context, msg agentcore.ConfigureNotification) error {
           notificationCh <- msg

           stored, err := target.LoadDesiredConfig(ctx, msg.Target)
           if err != nil {
               loadErrCh <- err
               return err
           }
           loadedCh <- stored
           return nil
       })

6. Start target.
7. Start controller.
8. Call controller.SubmitConfigure(ctx, agentcore.ConfigureCommand{...}).

Use fields:
- Version: "1.0"
- RPCID: "rpc-config-e2e-1"
- Target: "vyos"
- UUID: "cfg-e2e-1"
- Payload: json.RawMessage(`{"hostname":"router-1","interfaces":[]}`)
- Timestamp: time.Now().UTC()

9. Assert SubmitConfigure ack:
- non-nil
- Accepted == true
- RPCID preserved
- Target == "vyos"
- Subject == "cmd.configure.vyos"
- KVBucket == bucket
- KVKey == "desired.vyos"
- KVRevision > 0

10. Wait for configure notification.
11. Assert notification:
- RPCID preserved
- Target == "vyos"
- UUID == "cfg-e2e-1"
- CommandType == "configure"
- KVBucket == bucket
- KVKey == "desired.vyos"
- Timestamp not zero

12. Wait for loaded desired config from target.LoadDesiredConfig(...)
13. Assert loaded desired config:
- non-nil
- Bucket == bucket
- Key == "desired.vyos"
- Revision > 0
- Record.RPCID == "rpc-config-e2e-1"
- Record.Target == "vyos"
- Record.UUID == "cfg-e2e-1"
- Record.Payload equals submitted payload

14. Also assert no load error was received.

TC metadata:
- ID: TC-INTEGRATION-PHASE5-SEND-003
- Type: Positive

This test proves the complete public configure flow:

    controller SubmitConfigure(...)
      → store desired config in KV
      → publish configure notification
      → target RegisterConfigureHandler(...)
      → target LoadDesiredConfig(...)

Do not use raw NATS publish for this test.

---

## Test 4: PublishStatus reaches registered status handler

Suggested name:

    TestIntegrationPublishStatusReachesRegisteredStatusHandler

Flow:

1. Start real nats-server.
2. Create unique bucket.
3. Create observer/controller client A.
4. Create target client B.
5. Register status handler on observer before Start(ctx):

       observer.RegisterStatusHandler("vyos", func(ctx context.Context, msg agentcore.StatusEnvelope) error {
           received <- msg
           return nil
       })

6. Start observer.
7. Start target.
8. Call target.PublishStatus(ctx, agentcore.StatusEnvelope{...}).

Use fields:
- Version: "1.0"
- RPCID: "rpc-status-e2e-1"
- Target: "vyos"
- Status: "running"
- Stage: "startup"
- Payload: json.RawMessage(`{"ready":true}`)
- Timestamp: time.Now().UTC()

9. Wait for observer status handler.
10. Assert:
- RPCID preserved
- Target == "vyos"
- Status == "running"
- Stage == "startup"
- Payload preserved
- Timestamp not zero

TC metadata:
- ID: TC-INTEGRATION-PHASE5-SEND-004
- Type: Positive

This test proves public PublishStatus and public RegisterStatusHandler work together over real NATS.

---

## Optional combined full workflow test

If the above four focused tests are implemented cleanly and not too slow, add one combined workflow test.

Suggested name:

    TestIntegrationPublicActionResultRoundTrip

Flow:

1. Start real nats-server.
2. Create controller and worker clients.
3. Controller registers result handler.
4. Worker registers action handler.
5. Worker action handler receives action and immediately calls worker.PublishResult(...) with same RPCID.
6. Start both clients.
7. Controller calls SubmitAction(...).
8. Assert controller receives result with same RPCID.

This validates:

    controller SubmitAction(...)
      → worker RegisterActionHandler(...)
      → worker PublishResult(...)
      → controller RegisterResultHandler(...)

TC metadata:
- ID: TC-INTEGRATION-PHASE5-SEND-005
- Type: Positive

Add this only if it remains deterministic and does not duplicate too much of tests 1 and 2.

If added, it is very valuable because it proves real action/result round-trip correlation using public APIs only.

---

## Helper guidance

Use small helpers only if they improve readability.

Possible helpers:

    func newStartedIntegrationClient(t *testing.T, serverURL, bucket string) *agentcore.Client

But be careful:
- some tests need registration before Start(ctx)
- so do not hide too much behind helpers

Suggested helper:

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

Use existing project style if already present.

Channel and timeout style:
- use buffered channels of size 1 where appropriate
- use select with time.After or context.WithTimeout
- prefer a 5 second timeout for real NATS integration delivery
- avoid arbitrary sleeps

Example wait pattern:

    select {
    case got := <-received:
        // assert
    case <-time.After(5 * time.Second):
        t.Fatal("timed out waiting for message")
    }

For configure load error channel:

    select {
    case err := <-loadErrCh:
        t.Fatalf("LoadDesiredConfig returned error: %v", err)
    default:
    }

But avoid checking default before handler runs. Prefer receive loaded config first or fail on loadErrCh.

---

## Client setup guidance

Use the existing config helper:

    cfg := newIntegrationConfig(serverURL, bucket, true)

Create separate clients for sender and receiver.

Use distinct client names if config supports it and existing helper permits it.
If not, it is okay to use the helper as-is unless duplicate client names cause test issues.

Start order:
- register receiver handlers first
- start receiver
- start sender
- then call sender API

This avoids missing Core NATS messages.

Close order:
- defer Close(ctx) for each started client
- use bounded close contexts

---

## Avoid flaky behavior

Do not publish before the receiver has subscribed.

Existing `Start(ctx)` should activate subscriptions before returning, so this should be safe:

    receiver.RegisterActionHandler(...)
    receiver.Start(ctx)
    sender.Start(ctx)
    sender.SubmitAction(...)

Do not add sleeps unless there is an existing helper pattern requiring a short readiness wait.

If delivery is flaky, use a small poll/wait helper around `receiver.Health().ActiveSubscriptions` to confirm active subscription count before sending. Prefer this over arbitrary sleep.

Example optional helper:

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

Use only if needed.

---

## Test expectations after implementation

The integration suite should now cover both categories.

Existing receive-side integration:
- raw NATS → handler receive
- configure receive
- action receive
- result receive
- status receive
- malformed receive drop
- post-start registration
- reconnect restore

New public sender-to-receiver integration:
- public SubmitAction → public RegisterActionHandler
- public PublishResult → public RegisterResultHandler
- public SubmitConfigure → public RegisterConfigureHandler
- public LoadDesiredConfig from target after configure notification
- public PublishStatus → public RegisterStatusHandler
- optional public SubmitAction → public PublishResult round trip

---

## Important assertions

For all tests:
- assert `rpc_id` is preserved exactly
- assert target is preserved
- assert subject/ACK where returned
- assert payload is preserved by comparing JSON string or unmarshaled maps
- assert timestamp is not zero
- assert no unexpected error from public API call
- assert no handler timeout

For configure:
- assert KV bucket and key in notification match ack
- assert loaded desired config payload matches submitted configure payload
- assert revision is non-zero

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
2. Integration tests added with TC IDs
3. Which public sender-to-receiver flows are covered
4. Whether raw NATS publishing was avoided in the new tests
5. Whether production code changed
6. Commands run and results
7. Any skipped tests and why
8. Confirmation that public APIs did not change
9. Confirmation that no workload/business logic was added
10. Confirmation that Phase 6 observability was not added
