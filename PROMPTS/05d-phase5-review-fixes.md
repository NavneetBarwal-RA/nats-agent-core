Read the current PR/codebase carefully.

This is a corrective Phase 5 review-fix prompt.

Focus only on fixing the issues identified in the Phase 5 senior Go code review.

Do not redesign Phase 5.
Do not change public API signatures.
Do not add workload/business logic.
Do not change message contracts, JSON tags, or exported structs unless absolutely required for compilation.
Do not rewrite the session/client/registry architecture.
Keep the implementation small, idiomatic, and easy to review.

Relevant Phase 5 areas:
- agentcore/client.go
- agentcore/handlers_runtime.go
- agentcore/handlers_runtime_test.go
- internal/session/session.go
- internal/session/callbacks.go
- internal/session/types.go
- internal/session/subscription_hooks_test.go
- internal/registry/*
- internal/subjects/*
- tests/integration if integration test infrastructure already exists

The issues to fix are:

1. PH5-01: Unexpected NATS close leaves registry subscriptions marked active
2. PH5-02: User handlers are invoked with context.Background()
3. PH5-03: Callback binders can panic on nil *nats.Msg
4. PH5-04: Missing live NATS integration coverage for subscribe/reconnect restore

Prioritize PH5-01 first because it is the only High-severity correctness issue.

---

## PH5-01: Fix stale active subscriptions after unexpected NATS close

Problem:
If the underlying NATS connection closes unexpectedly outside Client.Close(ctx), the session closed callback updates session health, but the client subscription registry can still mark subscriptions as active.

Later, if the app calls Start(ctx) again, activation can skip those entries because activateRecord(...) returns early when:

    rec.Active && !force

This can leave the client connected but without real active subscriptions.

Required behavior:
- When the session receives an unexpected closed callback, the client must:
  - disable receive callbacks
  - clear active subscription handles from the registry
  - sync subscription health counters
  - preserve valid subscription intent for future Start(ctx)
- A later Start(ctx) must be able to reactivate those saved intents.
- Do not remove registered subscription intent on unexpected close.
- Do not require agents to re-register handlers.
- Do not call user handlers during shutdown/closed cleanup.
- Do not double-close or panic on stale NATS subscription handles.

Implementation guidance:
1. Add a closed-session hook to internal/session.Hooks if it does not already exist:
   - OnClosed func()

2. Add a method on session.Manager:
   - SetClosedHandler(fn func())

3. Update internal/session/callbacks.go:
   - onClosed should update session state under lock
   - copy the OnClosed hook while holding the lock
   - release the lock
   - call the hook outside the lock

Example shape:

    func (m *Manager) onClosed(_ *nats.Conn) {
        m.mu.Lock()
        m.setClosedLocked(nil)
        onClosed := m.hooks.OnClosed
        m.mu.Unlock()

        if onClosed != nil {
            onClosed()
        }
    }

4. Update agentcore.New(...):
   - after client construction, wire:
     runtime.SetReconnectHandler(client.onSessionReconnected)
     runtime.SetClosedHandler(client.onSessionClosed)

5. Add Client.onSessionClosed():
   - callbacksEnabled.Store(false)
   - clear/deactivate active subscriptions
   - preserve registry intent
   - sync health counters
   - log cleanup errors and send them to errorSink if configured

Suggested implementation:

    func (c *Client) onSessionClosed() {
        c.callbacksEnabled.Store(false)
        c.cancelHandlerContext()

        if err := c.deactivateAllSubscriptions("session_closed"); err != nil {
            c.logWarn("failed to clear active subscriptions after session closed", "error", err)
            if c.options.errorSink != nil {
                c.options.errorSink(err)
            }
        }
    }

If cancelHandlerContext does not exist yet, it will be added for PH5-02.

6. Add or update unit tests:
   - session Manager stores closed hook
   - invoking session closed hook calls the registered callback
   - client onSessionClosed disables callbacks
   - client onSessionClosed clears active subscription handles
   - client onSessionClosed preserves RegisteredSubscriptions
   - after onSessionClosed, ActiveSubscriptions is zero
   - after onSessionClosed, activation records are no longer active and can be reactivated by Start(ctx)

Do not rely on real NATS for these unit tests.

---

## PH5-02: Replace context.Background() in user handler dispatch with lifecycle context

Problem:
The public handler signatures accept context.Context, but callback dispatch currently invokes user handlers with context.Background().

This means already-running handlers cannot observe client shutdown/session close.

Required behavior:
- User handlers should receive a lifecycle context owned by Client.
- The lifecycle context should be active after successful Start(ctx).
- The lifecycle context should be canceled on:
  - Client.Close(ctx)
  - unexpected session closed callback
  - failed Start(ctx) activation cleanup if a context was created
- Existing public handler signatures must not change.
- Do not introduce per-message timeouts unless the existing config already clearly supports them.
- Do not store the incoming Start(ctx) directly as the handler context because it may be canceled immediately after Start returns.

Implementation guidance:
1. Add private fields to Client:

    handlerCtx    context.Context
    handlerCancel context.CancelFunc

Protect these fields with the existing Client.mu.

2. Add helper methods:

    func (c *Client) setHandlerContext()
    func (c *Client) cancelHandlerContext()
    func (c *Client) handlerContext() context.Context

Expected behavior:
- setHandlerContext creates context.WithCancel(context.Background()).
- If an old handlerCancel exists, cancel it before replacing.
- cancelHandlerContext cancels the current handler context and clears fields.
- handlerContext returns current context if set, otherwise context.Background().

3. Update Start(ctx):
- Do not create handler lifecycle context before subscription activation succeeds.
- After activateAllSubscriptions("start") succeeds:
  - call c.setHandlerContext()
  - then callbacksEnabled.Store(true)

Expected order:

    if err := c.startSession(ctx); err != nil {
        return err
    }

    if err := c.activateAllSubscriptions("start"); err != nil {
        c.callbacksEnabled.Store(false)
        c.cancelHandlerContext()
        cleanupErr := c.deactivateAllSubscriptionsWithOp("start_activation_cleanup")
        ...
        return err
    }

    c.setHandlerContext()
    c.callbacksEnabled.Store(true)
    return nil

4. Update Close(ctx):
- Before deactivating subscriptions:
  - callbacksEnabled.Store(false)
  - c.cancelHandlerContext()

5. Update onSessionClosed():
- callbacksEnabled.Store(false)
- c.cancelHandlerContext()
- clear active subscriptions

6. Update callConfigureHandler, callActionHandler, callResultHandler, callStatusHandler:
- replace context.Background() with c.handlerContext()

Example:

    return handler(c.handlerContext(), msg)

7. Add unit tests:
- successful Start creates handler context and callbacks receive a non-canceled context
- Close cancels the context received by an already-dispatched handler
- onSessionClosed cancels handler context
- failed Start activation leaves no active handler context
- callbacks still work after successful Start with the lifecycle context

Keep tests deterministic. Avoid sleeps. Use channels only when needed and always avoid goroutine leaks.

---

## PH5-03: Add nil *nats.Msg guards to callback binders

Problem:
bindConfigureCallback, bindActionCallback, bindResultCallback, and bindStatusCallback dereference msg.Data and msg.Subject without checking msg for nil.

NATS likely does not pass nil messages, but the callback binding layer should be robust and should not panic on malformed callback input.

Required behavior:
- If callback receives nil *nats.Msg:
  - do not panic
  - do not call user handler
  - log a warning if logger exists
  - increment existing failure/decode metric if metrics hook exists
- Keep behavior unchanged for valid, malformed JSON, and validation-failing messages.

Implementation guidance:
1. Add a small helper if it reduces duplication:

    func (c *Client) dropNilMessage(kind registry.Kind) bool {
        c.logWarn("dropping nil subscription message", "kind", string(kind))
        if c.options.metrics != nil {
            c.options.metrics.IncSubscribe(string(kind), "", "decode_failed")
        }
        return true
    }

Or simply add direct guards in each binder.

2. Add guard after callbacksEnabled check:

    if msg == nil {
        c.logWarn("dropping nil result message")
        if c.options.metrics != nil {
            c.options.metrics.IncSubscribe(string(registry.KindResult), "", "decode_failed")
        }
        return
    }

3. Add unit tests:
- nil configure message does not panic and does not call handler
- nil action message does not panic and does not call handler
- nil result message does not panic and does not call handler
- nil status message does not panic and does not call handler

Use defer/recover in tests only if needed to assert no panic.

---

## PH5-04: Add live NATS integration coverage for subscribe and reconnect restore

Problem:
Current tests are mostly unit tests with direct callback invocation or fake subscription handles. They do not prove real NATS subscribe delivery or reconnect restoration.

Required behavior:
Add integration tests only if the repo already has an integration test setup with real nats-server support.

If integration test infrastructure exists:
- add tests under the existing integration test package/location
- use the existing integration build tag style
- do not run integration tests as part of normal unit tests unless the repo already does that

If integration test infrastructure does not exist:
- do not invent a large new test framework in this patch
- add a small TODO/follow-up note in the Phase 5 prompt or README only if consistent with repo style
- clearly report that integration tests were not added because the infrastructure was not present

Preferred integration tests if infrastructure exists:

1. Register result handler before Start and receive real published result
- start real nats-server using existing helper
- create client
- RegisterResultHandler("vyos", handler)
- Start(ctx)
- publish JSON to result.vyos using a NATS connection
- assert handler receives ResultEnvelope
- assert rpc_id is unchanged

2. Register action handler before Start and receive real action command
- RegisterActionHandler("vyos", "trace", handler)
- Start(ctx)
- publish to cmd.action.vyos.trace
- assert handler receives ActionCommand with target/action/rpc_id/payload

3. Queue group smoke test if practical
- register two clients with same queue group
- publish one message
- assert exactly one handler receives it
- keep this test only if deterministic and reliable

4. Reconnect restore test if existing integration setup can restart or disconnect server reliably
- register handler
- Start(ctx)
- verify first message received
- force reconnect using existing helper
- wait for reconnect health if helper exists
- publish second message
- verify second message received without re-registering handler

Avoid flaky tests:
- use context deadlines
- avoid arbitrary sleeps where possible
- use channels with timeouts
- clean up clients and NATS connections
- skip reconnect test if existing helper cannot perform reliable restart/reconnect

---

## Important behavioral constraints

Do not break these existing Phase 5 behaviors:
- Register*Handler before Start(ctx) succeeds and stores intent.
- Register*Handler after successful Start(ctx) activates immediately.
- Failed immediate activation rolls back the newly added registry entry.
- Failed Start(ctx) activation leaves callbacks disabled and clears partial active handles.
- Reconnect restore uses saved registry intent.
- Result handler receives rpc_id unchanged.
- Subject building uses internal/subjects centralized builder.
- No raw subject string duplication should be reintroduced.
- No workload-specific logic should be added.

---

## Error handling expectations

Use existing typed error style:
- agentcore.Error for public-facing errors
- runtimeerr.Error for internal package errors
- convert internal errors with existing toPublicError helpers

Do not introduce string-only error matching in production code.

When cleanup fails:
- log warning if logger exists
- call errorSink if configured
- avoid masking the primary error unless existing code already joins cleanup errors

---

## Concurrency and locking expectations

- Do not hold session locks while calling client hooks.
- Do not hold client locks while invoking user handlers.
- Do not hold registry locks while invoking user handlers.
- Avoid holding locks while calling NATS operations when possible.
- Keep callbacksEnabled atomic usage consistent.
- Protect handler context fields with Client.mu.
- Avoid goroutine leaks in tests.

---

## Tests to update/add

Unit tests:
- internal/session/subscription_hooks_test.go:
  - SetClosedHandler stores callback
  - stored closed callback can be invoked safely

- agentcore/handlers_runtime_test.go:
  - onSessionClosed disables callbacks and clears active handles
  - onSessionClosed preserves registered intent
  - Start after onSessionClosed can reactivate saved intent using test seam
  - Close cancels handler lifecycle context
  - handler receives lifecycle context, not a nil context
  - nil *nats.Msg callback inputs do not panic or call handlers
  - existing Start activation failure cleanup tests still pass
  - existing registration rollback tests still pass

- internal/registry tests only if registry changes are required

Integration tests:
- add only if existing integration infrastructure exists
- real NATS result subscribe delivery
- real NATS action subscribe delivery
- reconnect restore if reliable helpers already exist

---

## Verification commands

Run:

    gofmt on all changed Go files
    go test ./...
    go test -race ./...

If integration tests are added, also run the repo’s existing integration command, for example:

    go test -count=1 -v -tags=integration ./tests/integration/...
    go test -count=1 -race -tags=integration ./tests/integration/...

Do not claim a command passed unless it was actually run.

---

## Final response required from Codex

After coding, summarize:

1. Files changed
2. How PH5-01 was fixed
3. How PH5-02 was fixed
4. How PH5-03 was fixed
5. Whether PH5-04 integration tests were added
6. Tests added or updated
7. Commands run and results
8. Confirmation that public APIs did not change
9. Confirmation that subject helper de-duplication was preserved
10. Confirmation that no workload/business logic was added
