Read the current codebase carefully before making changes.

This is a Phase 5 completion prompt.

The goal is to finish the currently unimplemented public sender/publisher facade APIs so Phase 5 provides full functional send + receive support for real agents.

Currently Phase 5 has completed the receive side:

- RegisterConfigureHandler(...)
- RegisterActionHandler(...)
- RegisterResultHandler(...)
- RegisterStatusHandler(...)
- subscribe wrappers
- callback binding
- subscription registry
- reconnect-safe restore
- receive-side rpc_id preservation

But the public send/publish facade APIs are still not implemented:

- SubmitConfigure(...)
- SubmitAction(...)
- PublishResult(...)
- PublishStatus(...)

These must be implemented in Phase 5 so the library can support full public agent-to-agent flows before Phase 6 observability work.

Do not redesign Phase 5.
Do not change public API signatures.
Do not change exported model names or JSON tags unless absolutely required for compilation.
Do not add workload/business logic.
Do not add observability redesign.
Do not add worker pools.
Do not add public unsubscribe/deregister APIs.
Do not add unit or integration tests in this prompt unless existing tests must be minimally adjusted because stale “not implemented” expectations fail.
A separate test prompt will be created after this implementation.

Relevant files to inspect first:

- SPEC.md
- REQUIREMENTS_CHECKLIST.md
- README.md
- agentcore/client.go
- agentcore/models.go
- agentcore/contracts.go
- agentcore/errors.go
- internal/transport/*
- internal/subjects/*
- internal/contract/*
- internal/kv/*
- internal/session/*
- existing tests only for understanding current behavior

Primary requirement:
After this implementation, none of these public APIs should return CodeNotImplemented under normal valid runtime conditions:

- SubmitConfigure(...)
- SubmitAction(...)
- PublishResult(...)
- PublishStatus(...)

They should use the existing session, subject, KV, transport, validation, and codec foundations already present in the repo.

---

## Scope

Implement code only for the public sender/publisher APIs.

Required public behavior:

1. SubmitConfigure(ctx, cmd)
   - validate input
   - store desired config in KV
   - publish configure notification on the configured configure subject
   - return a SubmissionAck

2. SubmitAction(ctx, cmd)
   - validate input
   - publish action command on the configured action subject
   - return a SubmissionAck

3. PublishResult(ctx, msg)
   - validate input
   - publish result envelope on the configured result subject
   - return error on failure

4. PublishStatus(ctx, msg)
   - validate input
   - publish status envelope on the configured status subject
   - return error on failure

Do not implement request/response waiting.
Do not implement result correlation store.
Do not block waiting for a result.
Do not add durable JetStream consumers.
Do not add business-specific action/configure logic.

This phase only needs to submit/publish messages correctly and expose success/failure clearly.

---

## Design expectations

Use the existing architecture:

- agentcore remains the public facade
- internal/session owns NATS connection lifecycle
- internal/subjects owns subject generation and validation
- internal/kv owns desired-config KV store/load/watch helpers
- internal/transport should own publish-specific transport operations if existing foundations exist
- internal/contract should own shared codec/validation if existing helpers exist

Avoid duplicating subject construction in agentcore.
Avoid duplicating encode/validate behavior if internal helpers already exist.
Keep target-first subject routing:

- configure → cmd.configure.<target>
- action → cmd.action.<target>.<action>
- result → result.<target>
- status → status.<target>

Use custom subject patterns from Config.Subjects through the centralized subjects builder.

---

## SubmitConfigure(ctx, cmd)

Current public method exists but returns CodeNotImplemented.

Implement it.

Expected behavior:

1. Validate ctx
   - if ctx is nil, return CodeValidation
   - if ctx is already canceled/deadlined, return the context error or a typed error wrapping it using current project conventions

2. Validate ConfigureCommand
   Required fields should follow existing model/spec conventions.
   Inspect current ConfigureCommand struct and existing validation helpers.
   Expected required fields likely include:
   - Version
   - RPCID
   - Target
   - UUID
   - Payload
   - Timestamp

   Do not invent incompatible fields.
   Follow existing contract/model validation rules.

3. Store desired config in KV
   - Convert ConfigureCommand into DesiredConfigRecord or the correct existing KV record type.
   - Store using existing public/internal KV helper.
   - Use the configured KV bucket/key pattern.
   - Preserve:
     - version
     - rpc_id
     - target
     - uuid
     - payload
     - timestamp

4. Publish configure notification
   - Build ConfigureNotification from the stored desired config.
   - Include:
     - Version
     - RPCID
     - Target
     - CommandType, likely "configure"
     - UUID
     - KVBucket
     - KVKey
     - Timestamp
   - Publish to:
     - cmd.configure.<target>
     - through internal/subjects builder, not hardcoded string logic
   - Use existing NATS connection from session.
   - Use existing timeout/retry behavior if internal/transport already provides it.
   - If no transport helper exists, implement a small internal helper consistent with existing style.

5. Return SubmissionAck
   Inspect the existing SubmissionAck model.
   Populate fields according to current model/spec.
   At minimum, it should preserve/return:
   - RPCID
   - Target
   - Subject if the model supports it
   - timestamp or accepted time if the model supports it
   - KV bucket/key/revision if the model supports it

6. Error behavior
   - validation errors return CodeValidation
   - disconnected/session errors return CodeDisconnected or current connection error code
   - KV store errors return the existing KV-related public error
   - publish errors return CodePublishFailed or existing publish failure code
   - preserve operation name, subject, key, retryability, and wrapped error where possible

Suggested op names:
- submit_configure
- submit_configure_store_desired
- submit_configure_publish_notification

Do not hide failures only in logs.

---

## SubmitAction(ctx, cmd)

Current public method exists but returns CodeNotImplemented.

Implement it.

Expected behavior:

1. Validate ctx
   - nil ctx rejected
   - canceled ctx handled clearly

2. Validate ActionCommand
   Required fields should follow existing model/spec conventions.
   Expected required fields likely include:
   - Version
   - RPCID
   - Target
   - CommandType
   - Action
   - Payload
   - Timestamp

3. Publish action command
   - Build subject using centralized subjects builder:
     - cmd.action.<target>.<action>
   - Encode using existing contract/codec helpers if available
   - Publish using existing internal transport/session helper
   - Preserve rpc_id exactly
   - Do not wait for a result

4. Return SubmissionAck
   Populate according to existing model/spec.
   At minimum, preserve/return:
   - RPCID
   - Target
   - Action if model supports it
   - Subject if model supports it
   - accepted/published timestamp if model supports it

5. Error behavior
   - validation failures are CodeValidation
   - no active session/connection returns CodeDisconnected or current connection error code
   - publish failure returns CodePublishFailed or current project equivalent
   - include subject where known

Suggested op names:
- submit_action
- submit_action_publish

---

## PublishResult(ctx, msg)

Current public method exists but returns CodeNotImplemented.

Implement it.

Expected behavior:

1. Validate ctx
   - nil ctx rejected
   - canceled ctx handled clearly

2. Validate ResultEnvelope
   Required fields should match receive-side validation already implemented.
   Expected required fields:
   - Version
   - RPCID
   - Target
   - Result
   - Timestamp

   Optional fields should remain optional:
   - CommandType
   - UUID
   - Action
   - ErrorCode
   - Payload

3. Publish result envelope
   - Build subject using centralized subjects builder:
     - result.<target>
   - Encode with existing codec/contract helpers if available
   - Publish through NATS session/transport
   - Preserve rpc_id exactly
   - Do not mutate or regenerate rpc_id

4. Return error only
   - nil on successful publish/flush according to existing publish semantics
   - typed error on validation/connection/publish failure

Suggested op names:
- publish_result
- publish_result_publish

---

## PublishStatus(ctx, msg)

Current public method exists but returns CodeNotImplemented.

Implement it.

Expected behavior:

1. Validate ctx
   - nil ctx rejected
   - canceled ctx handled clearly

2. Validate StatusEnvelope
   Required fields should match receive-side validation already implemented.
   Expected required fields:
   - Version
   - Target
   - Status
   - Timestamp

   Optional fields should remain optional:
   - RPCID
   - UUID
   - Stage
   - Payload

3. Publish status envelope
   - Build subject using centralized subjects builder:
     - status.<target>
   - Encode with existing codec/contract helpers if available
   - Publish through NATS session/transport
   - Preserve optional rpc_id exactly if provided
   - Do not require rpc_id for status if the receive-side model treats it as optional

4. Return error only
   - nil on successful publish/flush
   - typed error on validation/connection/publish failure

Suggested op names:
- publish_status
- publish_status_publish

---

## Transport implementation guidance

Inspect internal/transport before implementing.

If internal/transport already has publish helpers:
- reuse them
- expose only minimal internal constructors/functions needed by agentcore
- do not duplicate lower-level publish code in agentcore

If internal/transport publish helpers exist but are not connected:
- wire agentcore public methods to them
- keep internal package boundaries clean

If no suitable helper exists:
- add a small focused internal transport publisher implementation
- it should:
  - accept a NATS connection or session-backed connection provider
  - encode payloads consistently
  - call Publish or Request only if spec requires it
  - call Flush or FlushWithContext if existing publish semantics require delivery confirmation
  - respect ctx/timeouts where possible
  - return typed internal/runtime errors converted to public errors by agentcore

Do not create a large framework.
Do not add durable/JetStream publishing unless the spec already says so.
Core NATS publish is expected for action/result/status.
Configure should use store-then-notify:
- KV store for desired config
- Core NATS notify on configure subject

---

## Context and timeout behavior

Use ctx from public API calls.

Expected behavior:
- if ctx is nil, return validation error
- if ctx is canceled before work starts, return clearly
- pass ctx into KV store and publish helpers where supported
- respect configured publish timeout if existing transport/session code already does
- avoid blocking forever on NATS flush

If NATS publish helper cannot accept context directly:
- use FlushTimeout or existing configured timeout behavior
- document by code structure, not README in this prompt

Do not store caller ctx for later use.
Do not use context.Background() for the actual public operation unless no alternative exists and existing project style already does so.

---

## Validation guidance

Prefer existing shared validation.

Avoid inconsistent duplicate validation between send and receive paths.

If receive-side validators exist in agentcore/handlers_runtime.go and are suitable:
- consider moving shared validators into a common file/package if needed
- avoid import cycles
- keep validators unexported unless public export is already intended

Validation should reject:
- missing/whitespace required strings
- zero required timestamps
- invalid required JSON payloads
- invalid optional JSON payloads
- invalid target/action tokens through internal/subjects helpers

Do not accidentally make status rpc_id required if receive-side currently allows it as optional.

---

## SubmissionAck guidance

Inspect current SubmissionAck model before filling it.

Populate only fields that exist.

Expected intent:
- caller should get clear success outcome
- ack should include correlation fields where the model supports them
- ack should not claim downstream handler execution
- ack should mean “accepted/published to bus” or “stored + notify published” for configure

Do not change SubmissionAck public shape unless the existing shape makes successful ack impossible. If a shape change is unavoidable, keep it minimal and consistent with SPEC.md.

---

## Error handling expectations

Use existing typed public errors:
- agentcore.Error
- CodeValidation
- CodeDisconnected
- CodePublishFailed if available
- CodeKVFailed or equivalent if available
- CodeNotImplemented should no longer be used by these four APIs under valid implemented conditions

Use existing internal runtime errors where appropriate:
- runtimeerr.Error
- convert with toPublicError(...)

Do not return raw NATS errors directly from public APIs if current style wraps them.

Each error should include useful fields where possible:
- Op
- Subject
- Key
- Message
- Retryable
- Err

Do not only log failures.
Return failures to the caller.

---

## Lifecycle expectations

Public send APIs should require an active started session.

Expected:
- calling send/publish before Start(ctx) should return a clear disconnected/not-started error
- calling after Close(ctx) should return a clear disconnected/closed error
- calling during reconnect/degraded state should follow existing session.Connection() behavior
- do not auto-start the client from send APIs

Do not use registry/callback state for publishing.
Publishing should depend on session connection availability.

---

## Observability expectations

Do not redesign observability in this prompt.

If existing metrics hooks are already used by transport publish code:
- keep them consistent
- do not add new public metrics methods

Do not add ObserveSubscribeLatency or similar here.
That belongs to Phase 6.

---

## README / prompts / requirement docs

Do not update README or requirement docs in this prompt unless needed to remove obviously false statements caused by code implementation.

The user will update README, PR description, and requirement files separately.

Focus on production code implementation.

---

## Tests

Do not add the new unit/integration test suite in this prompt.

A separate prompt will cover:
- unit tests for public send APIs
- integration tests for sender-to-receiver flows
- full configure/action/result/status end-to-end tests

However:
- If existing tests fail because they still expect CodeNotImplemented for these APIs, update only those stale tests minimally.
- Do not leave the repo failing to compile.

---

## Verification commands

Run:

    gofmt on all changed Go files
    go test ./...

If feasible, also run:

    go test -race ./...

Do not claim a command passed unless it was actually run.

If tests fail because explicit new tests are not yet written, that is not acceptable.
Existing tests should pass or be minimally updated for the new implemented behavior.

---

## Expected result

After implementation:

- SubmitConfigure(...) is implemented
- SubmitAction(...) is implemented
- PublishResult(...) is implemented
- PublishStatus(...) is implemented
- no public facade send/publish API still returns CodeNotImplemented for valid use
- send APIs use centralized subject helpers
- configure uses KV store-then-notify
- action uses direct action publish
- result/status use direct result/status publish
- rpc_id is preserved
- public APIs return clear errors
- no workload/business logic is added
- Phase 6 observability scope remains separate

---

## Final response required from Codex

After coding, summarize:

1. Files changed
2. How SubmitConfigure was implemented
3. How SubmitAction was implemented
4. How PublishResult was implemented
5. How PublishStatus was implemented
6. Whether internal/transport helpers were reused or added
7. Validation behavior added/reused
8. Error behavior
9. Any minimal existing test updates
10. Commands run and results
11. Confirmation that public API signatures did not change
12. Confirmation that no workload/business logic was added
13. Confirmation that observability redesign was not added
