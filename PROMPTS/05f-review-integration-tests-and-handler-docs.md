Read the current PR/codebase carefully.

This is a Phase 5 review-fix prompt.

The reviewer requested additional integration coverage for Phase 5 receive-side handlers and a short documentation note about handler execution behavior.

Focus only on:
1. adding the missing Phase 5 integration tests requested in PR review
2. adding a small README/API documentation note explaining inline handler execution

Do not redesign Phase 5.
Do not change public API signatures.
Do not add new production features.
Do not change message contracts, JSON tags, exported structs, or handler signatures.
Do not change registry behavior, reconnect behavior, callback binding behavior, or subject builder behavior unless a test exposes a real bug.
Do not add workload/business logic.
Do not implement worker pools or async handler execution in the library.

Relevant existing files:
- tests/integration/phase5_subscribe_reconnect_integration_test.go
- tests/integration/testserver_test.go
- README.md
- agentcore/handlers_runtime.go only if a real test-discovered bug must be fixed

Existing integration test infrastructure:
- integration tests use build tag:
  - //go:build integration
  - // +build integration
- tests use startTestNATSServer(t)
- tests use newIntegrationConfig(serverURL, bucket, autoCreate)
- tests use uniqueName(prefix)
- tests use a real nats-server -js
- tests should skip if nats-server is not available, using existing helper behavior
- tests should use real NATS publish/subscribe behavior
- tests should use context deadlines or channel timeouts
- tests should avoid arbitrary sleeps except where existing helper patterns already use polling
- tests should clean up clients and NATS connections

Reviewer requested integration tests:

1. Configure handler real delivery
   - register RegisterConfigureHandler(...)
   - publish to cmd.configure.<target>
   - verify handler receives rpc_id, uuid, kv_bucket, kv_key

2. Status handler real delivery
   - register RegisterStatusHandler(...)
   - publish to status.<target>
   - verify handler receives target, status, optional rpc_id

3. Post-start registration delivery
   - call Start(ctx) first
   - then register a handler
   - publish a message
   - verify immediate activation works

4. Malformed message is dropped
   - register a real handler
   - publish malformed JSON or missing required field
   - verify handler is not invoked

Also address reviewer documentation request:
- handlers currently execute inline inside the NATS subscription callback
- long-running/blocking handlers can delay subsequent messages on that subscription
- document that agents should offload heavy work to goroutines, worker pools, or internal queues when needed
- keep this as documentation only; do not change runtime execution model in this PR

---

## Integration test 1: configure handler real delivery

Add a test under tests/integration/phase5_subscribe_reconnect_integration_test.go.

Suggested name:

    TestIntegrationConfigureHandlerReceivesRealPublishedConfigure

Test behavior:
1. Start real nats-server using startTestNATSServer(t)
2. Create unique KV bucket name
3. Create client with newIntegrationConfig(srv.URL, bucket, true)
4. Defer client.Close(context.Background())
5. Register configure handler before Start(ctx):

       client.RegisterConfigureHandler("vyos", func(ctx context.Context, msg agentcore.ConfigureNotification) error {
           received <- msg
           return nil
       })

6. Start client
7. Create ConfigureNotification payload with:
   - Version: "1.0"
   - RPCID: "rpc-cfg-live-1"
   - Target: "vyos"
   - CommandType: "configure"
   - UUID: "cfg-live-1"
   - KVBucket: bucket
   - KVKey: "desired.vyos"
   - Timestamp: time.Now().UTC()

8. Marshal payload to JSON
9. Create publisher connection with nats.Connect(srv.URL, nats.NoReconnect())
10. Publish to:

       cmd.configure.vyos

11. Flush publisher
12. Wait on channel with timeout
13. Assert received message preserves:
   - RPCID
   - UUID
   - KVBucket
   - KVKey
   - Target
   - CommandType

Use the existing integration-test style and timeout pattern.

---

## Integration test 2: status handler real delivery

Add a test under tests/integration/phase5_subscribe_reconnect_integration_test.go.

Suggested name:

    TestIntegrationStatusHandlerReceivesRealPublishedStatus

Test behavior:
1. Start real nats-server
2. Create client
3. Register status handler before Start(ctx):

       client.RegisterStatusHandler("vyos", func(ctx context.Context, msg agentcore.StatusEnvelope) error {
           received <- msg
           return nil
       })

4. Start client
5. Create StatusEnvelope payload with:
   - Version: "1.0"
   - RPCID: "rpc-status-live-1"
   - Target: "vyos"
   - Status: "running"
   - Stage: optional, e.g. "startup"
   - Timestamp: time.Now().UTC()

6. Marshal payload
7. Publish to:

       status.vyos

8. Flush
9. Assert handler receives:
   - Target == "vyos"
   - Status == "running"
   - RPCID == "rpc-status-live-1"
   - Stage if set

This proves status subscription wrapper and callback binding work with a real NATS message.

---

## Integration test 3: post-start registration delivery

Add a test under tests/integration/phase5_subscribe_reconnect_integration_test.go.

Suggested name:

    TestIntegrationPostStartRegistrationReceivesPublishedMessage

Test behavior:
1. Start real nats-server
2. Create client
3. Start client before registering the handler:

       if err := client.Start(context.Background()); err != nil {
           t.Fatalf(...)
       }

4. Register a handler after Start(ctx).
   Prefer result handler because payload is simple:

       client.RegisterResultHandler("vyos", func(ctx context.Context, msg agentcore.ResultEnvelope) error {
           received <- msg
           return nil
       })

5. Publish ResultEnvelope to:

       result.vyos

6. Assert handler receives the message
7. Verify:
   - RPCID preserved
   - Target preserved
   - Result preserved

This proves Register*Handler after Start(ctx) immediately activates the subscription.

Important:
- The test should fail if post-start registration only stores intent but does not subscribe immediately.
- Do not call Start(ctx) again after registering.
- Do not manually activate registry internals.

---

## Integration test 4: malformed message is dropped

Add a test under tests/integration/phase5_subscribe_reconnect_integration_test.go.

Suggested name:

    TestIntegrationMalformedMessageIsDroppedWithoutHandlerInvocation

Test behavior:
1. Start real nats-server
2. Create client
3. Register a real handler before Start(ctx)
   Prefer result handler:

       received := make(chan agentcore.ResultEnvelope, 1)
       client.RegisterResultHandler("vyos", func(ctx context.Context, msg agentcore.ResultEnvelope) error {
           received <- msg
           return nil
       })

4. Start client
5. Publish malformed JSON to:

       result.vyos

   Example malformed payload:

       []byte("{")

6. Flush publisher
7. Assert handler is not invoked within a short timeout.

Use a reasonable timeout that avoids flakiness, for example 500ms to 1s.

8. Then publish a valid ResultEnvelope to the same subject.
9. Assert handler receives the valid message.

This proves:
- malformed message is dropped
- handler is not invoked for invalid payload
- subscription remains healthy after malformed payload

Optional:
- Instead of malformed JSON, or in addition, publish JSON missing required `rpc_id`.
- Keep the test simple and deterministic.
- Do not rely on logs for assertion.

Recommended pattern:

    select {
    case got := <-received:
        t.Fatalf("expected malformed message to be dropped, got callback: %+v", got)
    case <-time.After(750 * time.Millisecond):
        // expected
    }

Then publish valid payload and wait up to 5 seconds.

---

## Helper usage

If repeated publisher code becomes too noisy, add a small helper in the integration test file, such as:

    func publishRaw(t *testing.T, serverURL, subject string, data []byte)

or:

    func publishJSON(t *testing.T, serverURL, subject string, value any)

Guidelines for helpers:
- Keep helpers test-local in tests/integration/phase5_subscribe_reconnect_integration_test.go unless useful across multiple integration files.
- Use t.Helper()
- Connect with nats.NoReconnect()
- Defer pub.Close()
- Check Publish error
- Check Flush error

Example helper:

    func publishRaw(t *testing.T, serverURL, subject string, data []byte) {
        t.Helper()

        pub, err := nats.Connect(serverURL, nats.NoReconnect())
        if err != nil {
            t.Fatalf("failed to connect publisher: %v", err)
        }
        defer pub.Close()

        if err := pub.Publish(subject, data); err != nil {
            t.Fatalf("Publish returned unexpected error: %v", err)
        }
        if err := pub.Flush(); err != nil {
            t.Fatalf("Flush returned unexpected error: %v", err)
        }
    }

If adding publishJSON helper:

    func publishJSON(t *testing.T, serverURL, subject string, value any) {
        t.Helper()

        raw, err := json.Marshal(value)
        if err != nil {
            t.Fatalf("failed to encode payload: %v", err)
        }
        publishRaw(t, serverURL, subject, raw)
    }

Use helpers only if they improve readability.

---

## README documentation update

Update README.md with a short section explaining handler execution behavior.

Suggested section title:

    Handler execution model

Suggested content:

    Registered configure/action/result/status handlers are executed inline from the NATS subscription callback.

    Handlers should return quickly. Long-running or blocking work should be offloaded by the agent to a goroutine, worker pool, or internal job queue. This prevents later messages on the same subscription from being delayed and avoids backpressure inside the NATS client callback path.

    The library owns subscription registration, callback binding, envelope decoding, and reconnect restore. Agents remain responsible for workload execution and handler concurrency policy.

Keep wording concise and practical.

Do not imply the library provides worker pools.
Do not imply handlers are executed concurrently by default.
Do not add a new concurrency API in this PR.

---

## Test expectations

After implementation, the Phase 5 integration coverage should include:

Existing integration tests:
- result handler receives real NATS result messages
- action handler receives real NATS action command messages
- reconnect restore delivers after server restart

New integration tests:
- configure handler receives real NATS configure notification
- status handler receives real NATS status envelope
- post-start handler registration immediately activates and receives message
- malformed message is dropped without invoking handler, and valid message still works afterward

---

## Verification commands

Run:

    gofmt on changed Go files
    go test ./...
    go test -race ./...
    go test -count=1 -v -tags=integration ./tests/integration/...
    go test -count=1 -race -tags=integration ./tests/integration/...

If integration tests are skipped because nats-server is not installed, say that clearly.
Do not claim integration tests passed unless they actually ran.

---

## Final response required from Codex

After coding, summarize:

1. Files changed
2. Integration tests added
3. README/API documentation added
4. Whether any production code changed
5. Commands run and results
6. Any skipped tests and why
7. Confirmation that public APIs did not change
8. Confirmation that handler runtime behavior was not redesigned
9. Confirmation that no workload/business logic was added
