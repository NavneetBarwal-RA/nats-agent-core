Read the current codebase carefully before making changes.

This is a Phase 5 unit-test prompt for the newly implemented public send/publish facade APIs.

The public send APIs are now implemented and transport publish mechanics were refactored into internal/transport.Publisher.

Now add proper unit coverage for the public facade methods:

- SubmitConfigure(...)
- SubmitAction(...)
- PublishResult(...)
- PublishStatus(...)

Do not add integration tests in this prompt.
Do not update README or PR description in this prompt.
Do not change public API signatures.
Do not change public model structs or JSON tags.
Do not change production behavior.
Do not add workload/business logic.
Do not add Phase 6 observability redesign.

Use the repository’s existing test-case comment standard from:

- PROMPTS/TESTS_STANDARD_TEMPLATE.md

Every new test function must include a TC-* block comment matching the existing format:

/*
TC-CLIENT-SEND-001
Type: Positive
Title: <short title>
Summary:
<summary>

Validates:
  - <specific behavior>
  - <specific behavior>
*/

Use clear sequential IDs:

- TC-CLIENT-SEND-001
- TC-CLIENT-SEND-002
- TC-CLIENT-SEND-003
- etc.

Place tests in the most appropriate existing public facade test file, likely:

- agentcore/client_test.go

If the file becomes too large, it is acceptable to create:

- agentcore/client_send_test.go

Preferred: create agentcore/client_send_test.go to keep send-api tests focused.

---

## Important implementation note: private test seam may be needed

Current Client uses concrete fields:

- session *session.Manager
- kv *kv.Store
- publisher *transport.Publisher

For unit tests, do not use a real NATS server and do not use a real JetStream KV store.

If required, add small private test seams only.

Acceptable private seams:

1. A private publisher interface in agentcore:

    type publisher interface {
        Publish(ctx context.Context, op, kind, subject string, payload []byte) error
    }

Then change Client.publisher from:

    publisher *transport.Publisher

to:

    publisher publisher

or similar unexported interface name.

transport.Publisher should satisfy it.

2. Optional private function seam for desired config storage if needed:

    storeDesiredConfigFn func(context.Context, DesiredConfigRecord) (*StoredDesiredConfig, error)

Then StoreDesiredConfig or SubmitConfigure can use this test seam internally.

However:
- Do not expose these seams publicly.
- Do not change public APIs.
- Do not change runtime behavior.
- Keep seams minimal and idiomatic.
- Prefer using existing seam patterns already present in Client, such as startSessionFn, activateAllSubscriptionsFn, and deactivateAllSubscriptionsFn.

The purpose is to unit-test facade behavior without requiring a real NATS server or JetStream KV.

---

## Required fake helpers

Add test-local fakes in agentcore/client_send_test.go.

Suggested fake publisher:

    type fakePublisher struct {
        calls []publishCall
        err   error
    }

    type publishCall struct {
        ctx     context.Context
        op      string
        kind    string
        subject string
        payload []byte
    }

    func (p *fakePublisher) Publish(ctx context.Context, op, kind, subject string, payload []byte) error {
        p.calls = append(p.calls, publishCall{
            ctx: ctx,
            op: op,
            kind: kind,
            subject: subject,
            payload: append([]byte(nil), payload...),
        })
        return p.err
    }

Suggested helper to create a send-test client:

    func newSendTestClient(t *testing.T) (*Client, *fakePublisher) {
        t.Helper()

        client, err := New(testConfig())
        if err != nil {
            t.Fatalf("New returned unexpected error: %v", err)
        }

        pub := &fakePublisher{}
        client.publisher = pub

        return client, pub
    }

If SubmitConfigure needs KV storage seam:

    func installStoreDesiredConfigStub(client *Client, stored *StoredDesiredConfig, err error) {
        client.storeDesiredConfigFn = func(context.Context, DesiredConfigRecord) (*StoredDesiredConfig, error) {
            return stored, err
        }
    }

Use the exact seam names that best fit the production code.

---

## Required unit tests

### 1. SubmitAction publishes valid action and returns ACK

Suggested name:

    TestSubmitActionPublishesCommandAndReturnsAck

Behavior:
- create client with fake publisher
- make session appear connected if required by current implementation
  - if the public method calls session.Connection() before publisher, either:
    - add a small private connection-check seam, or
    - rely on publisher.Connection behavior if refactored
  - Do not use real NATS in unit tests.
- call SubmitAction(ctx, ActionCommand{...})
- assert ack is non-nil
- assert ack.Accepted is true
- assert ack.RPCID matches input
- assert ack.Target matches input
- assert ack.Subject == "cmd.action.vyos.trace"
- assert fake publisher got exactly one call
- assert call.op == "submit_action_publish"
- assert call.kind == "action"
- assert call.subject == "cmd.action.vyos.trace"
- unmarshal call.payload into ActionCommand
- assert rpc_id, target, action, command_type, payload, timestamp are preserved

TC metadata:
- ID: TC-CLIENT-SEND-001
- Type: Positive

---

### 2. SubmitAction rejects invalid input before publish

Suggested name:

    TestSubmitActionRejectsInvalidInputBeforePublish

Behavior:
- create client with fake publisher
- call SubmitAction with invalid command, for example missing rpc_id or blank action
- assert error code CodeValidation
- assert ack is nil
- assert fake publisher received zero calls

TC metadata:
- ID: TC-CLIENT-SEND-002
- Type: Negative

---

### 3. SubmitAction converts publish failure to public error

Suggested name:

    TestSubmitActionReturnsPublishFailure

Behavior:
- create client with fake publisher returning runtimeerr.Error{
      Code: runtimeerr.CodePublishFailed,
      Op: "submit_action_publish",
      Subject: "cmd.action.vyos.trace",
      Message: "publish failed",
      Retryable: true,
  }
- call SubmitAction with valid command
- assert ack is nil
- assert error is *agentcore.Error
- assert Code == CodePublishFailed
- assert Op == "submit_action_publish"
- assert Subject == "cmd.action.vyos.trace"

TC metadata:
- ID: TC-CLIENT-SEND-003
- Type: Negative

---

### 4. PublishResult publishes valid result envelope

Suggested name:

    TestPublishResultPublishesEnvelope

Behavior:
- create client with fake publisher
- call PublishResult(ctx, ResultEnvelope{...})
- assert no error
- assert fake publisher got one call
- assert call.op == "publish_result"
- assert call.kind == "result"
- assert call.subject == "result.vyos"
- unmarshal call.payload into ResultEnvelope
- assert rpc_id, target, result, command_type, uuid, action, payload, timestamp are preserved

TC metadata:
- ID: TC-CLIENT-SEND-004
- Type: Positive

---

### 5. PublishResult rejects invalid envelope before publish

Suggested name:

    TestPublishResultRejectsInvalidEnvelopeBeforePublish

Behavior:
- create client with fake publisher
- call PublishResult with missing rpc_id or result
- assert error code CodeValidation
- assert fake publisher got zero calls

TC metadata:
- ID: TC-CLIENT-SEND-005
- Type: Negative

---

### 6. PublishStatus publishes valid status envelope with optional rpc_id

Suggested name:

    TestPublishStatusPublishesEnvelopeWithOptionalRPCID

Behavior:
- create client with fake publisher
- call PublishStatus with StatusEnvelope containing rpc_id
- assert no error
- assert subject == "status.vyos"
- unmarshal payload
- assert rpc_id is preserved
- assert target, status, stage, payload, timestamp are preserved

TC metadata:
- ID: TC-CLIENT-SEND-006
- Type: Positive

---

### 7. PublishStatus allows missing rpc_id

Suggested name:

    TestPublishStatusAllowsMissingRPCID

Behavior:
- create client with fake publisher
- call PublishStatus with valid status envelope where RPCID == ""
- assert no error
- assert fake publisher got one call
- unmarshal payload
- assert decoded RPCID == ""

This is important because status rpc_id is optional.

TC metadata:
- ID: TC-CLIENT-SEND-007
- Type: Positive

---

### 8. PublishStatus rejects invalid envelope before publish

Suggested name:

    TestPublishStatusRejectsInvalidEnvelopeBeforePublish

Behavior:
- call PublishStatus with missing target or status
- assert CodeValidation
- assert fake publisher got zero calls

TC metadata:
- ID: TC-CLIENT-SEND-008
- Type: Negative

---

### 9. SubmitConfigure stores desired config, publishes notification, and returns ACK

Suggested name:

    TestSubmitConfigureStoresDesiredConfigPublishesNotificationAndReturnsAck

Behavior:
- create client with fake publisher
- install fake StoreDesiredConfig behavior through a private seam if needed
- input ConfigureCommand:
  - Version: "1.0"
  - RPCID: "rpc-config-1"
  - Target: "vyos"
  - UUID: "cfg-1"
  - Payload: {"hostname":"router-1"}
  - Timestamp: fixed time
- fake store returns StoredDesiredConfig:
  - Bucket: "cfg_desired"
  - Key: "desired.vyos"
  - Revision: 42
  - CreatedAt: fixed time
- call SubmitConfigure
- assert ack:
  - Accepted true
  - RPCID "rpc-config-1"
  - Target "vyos"
  - Subject "cmd.configure.vyos"
  - KVBucket "cfg_desired"
  - KVKey "desired.vyos"
  - KVRevision 42
- assert publisher called once
- assert call.op == "submit_configure_publish_notification"
- assert call.kind == "configure"
- assert call.subject == "cmd.configure.vyos"
- unmarshal payload into ConfigureNotification
- assert:
  - RPCID preserved
  - Target preserved
  - UUID preserved
  - KVBucket/KVKey match stored result
  - CommandType == "configure"

TC metadata:
- ID: TC-CLIENT-SEND-009
- Type: Positive

---

### 10. SubmitConfigure returns KV store failure and does not publish

Suggested name:

    TestSubmitConfigureReturnsStoreFailureWithoutPublishing

Behavior:
- fake store returns agentcore.Error or runtime-converted error with CodeKVStoreFailed
- call SubmitConfigure
- assert ack nil
- assert error code CodeKVStoreFailed
- assert publisher calls zero

TC metadata:
- ID: TC-CLIENT-SEND-010
- Type: Negative

---

### 11. SubmitConfigure returns publish failure after successful store

Suggested name:

    TestSubmitConfigureReturnsPublishFailureAfterStore

Behavior:
- fake store succeeds
- fake publisher returns runtimeerr.CodePublishFailed
- call SubmitConfigure
- assert ack nil
- assert error code CodePublishFailed
- assert publisher got one call
- store was called once

TC metadata:
- ID: TC-CLIENT-SEND-011
- Type: Negative

---

### 12. Public send APIs reject nil and canceled contexts

Suggested name:

    TestPublicSendAPIsRejectNilAndCanceledContexts

Behavior:
- table-test all four APIs:
  - SubmitConfigure
  - SubmitAction
  - PublishResult
  - PublishStatus
- nil ctx should return CodeValidation
- canceled ctx should return CodeValidation
- fake publisher should not be called
- fake store should not be called for SubmitConfigure when ctx invalid

TC metadata:
- ID: TC-CLIENT-SEND-012
- Type: Negative

---

### 13. Public send APIs return disconnected before Start

This may already be partially covered in client_test.go.

If already covered sufficiently, do not duplicate.
If not complete, add or extend a focused test.

Behavior:
- create normal client using New(testConfig())
- do not start it
- call all four send APIs with valid inputs
- assert CodeDisconnected
- SubmitConfigure should not return ack
- SubmitAction should not return ack

TC metadata if new:
- ID: TC-CLIENT-SEND-013
- Type: Negative

---

## Important note about session connection check

Current implementation may explicitly call:

    c.session.Connection()

before calling c.publisher.Publish(...).

That makes success-path unit tests hard without a real connection.

Preferred cleanup:
- Move active connection checking fully inside internal/transport.Publisher.
- Public methods should rely on c.publisher.Publish(...) to return the disconnected error.
- This keeps transport ownership clean and improves unit testability.

If you do this:
- Ensure disconnected-before-start tests still pass.
- Ensure public behavior remains the same.
- Ensure errors are still converted using toPublicError(...).

Do not introduce a public API change.

Alternative:
- Add a private `connectionReadyFn` seam only for tests.

Preferred option is to avoid duplicate connection checks in public send methods and rely on publisher.

---

## Assertions and helpers

Use existing helper:

    requireErrorCode(t, err, CodeValidation)

or add a small helper if needed.

Use JSON unmarshal to verify encoded payloads.

Do not compare raw JSON strings unless field order is guaranteed.
Prefer unmarshalling payload into the expected model and comparing fields.

Use fixed times:

    fixedNow := time.Unix(1700000000, 0).UTC()

Use WithClock(fixedNowFunc) where accepted timestamp needs to be deterministic.

---

## Test style requirements

- Every test must have TC-* block comment.
- IDs must be unique.
- Use table tests only when it improves clarity.
- Keep test names descriptive.
- Do not make tests depend on real NATS.
- Do not make tests depend on real JetStream.
- Do not sleep.
- Do not add integration build tags.
- Keep tests deterministic.

---

## Verification commands

Run:

    gofmt on changed Go files
    go test ./agentcore
    go test ./internal/transport
    go test ./...

If feasible, also run:

    go test -race ./...

Do not claim a command passed unless it was actually run.

---

## Final response required from Codex

After coding, summarize:

1. Files changed
2. Private test seams added, if any
3. Unit tests added with TC IDs
4. What SubmitConfigure coverage now verifies
5. What SubmitAction coverage now verifies
6. What PublishResult coverage now verifies
7. What PublishStatus coverage now verifies
8. Whether production behavior changed
9. Whether public APIs changed
10. Commands run and results
11. Confirmation that no integration tests were added
12. Confirmation that no workload/business logic was added
