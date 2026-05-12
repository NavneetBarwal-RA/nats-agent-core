Read the current Phase 5 implementation carefully.

This is a small corrective prompt for the remaining Phase 5 lifecycle/context issue found during review.

Fix only the handler lifecycle context behavior around repeated Start(ctx).

Do not redesign Phase 5.
Do not change public API signatures.
Do not add workload/business logic.
Do not change message contracts, JSON tags, exported structs, or handler signatures.
Do not rewrite subscription registry, reconnect restore, callback binding, or subject helper logic.
Keep the patch small, focused, idiomatic, and easy to review.

Relevant files:
- agentcore/client.go
- agentcore/handlers_runtime.go
- agentcore/handlers_runtime_test.go

Background:
Phase 5 originally dispatched handlers with context.Background().
That was improved by adding a client-owned lifecycle context:

- Client.handlerCtx
- Client.handlerCancel
- setHandlerContext()
- cancelHandlerContext()
- handlerContext()

Handlers now receive c.handlerContext(), which is good.

Remaining issue:
Client.Start(ctx) currently calls setHandlerContext() after subscription activation succeeds.

Current behavior:
- Start(ctx) calls c.startSession(ctx)
- Start(ctx) calls c.activateAllSubscriptions("start")
- Start(ctx) calls c.setHandlerContext()
- Start(ctx) enables callbacks

The problem is setHandlerContext() cancels any existing handler context before creating a new one.

This means repeated Start(ctx) on an already-running client can cancel the active handler context even though the client remains running.

Why this matters:
The session layer treats Start(ctx) as idempotent when already connected.
A caller may call Start(ctx) more than once.
That should not cancel currently running handler work.
Handler contexts should be canceled only when the client is closing, the session closes unexpectedly, or a real lifecycle reset occurs.

Required behavior:
- Repeated Start(ctx) on an already-running client must not cancel the active handler context.
- Start(ctx) should create a handler lifecycle context only if:
  - no handler context exists, or
  - the existing handler context is already canceled
- Start(ctx) should preserve an existing active handler context.
- Close(ctx) must still cancel the handler context.
- onSessionClosed() must still cancel the handler context.
- Failed Start(ctx) activation must still leave no active handler context.
- Successful first Start(ctx) must still create a handler context.
- Handler dispatch must continue using c.handlerContext().
- callbacksEnabled behavior must remain unchanged:
  - false before successful Start activation
  - true after successful Start activation
  - false after Close(ctx)
  - false after onSessionClosed()

Implementation guidance:
1. Rename setHandlerContext() to ensureHandlerContext(), or add ensureHandlerContext() and stop using setHandlerContext() from Start(ctx).

Preferred helper behavior:

    func (c *Client) ensureHandlerContext() {
        c.mu.Lock()
        defer c.mu.Unlock()

        if c.handlerCtx != nil {
            select {
            case <-c.handlerCtx.Done():
                // Existing context is already canceled. Replace it below.
            default:
                // Existing context is active. Keep it.
                return
            }
        }

        ctx, cancel := context.WithCancel(context.Background())
        c.handlerCtx = ctx
        c.handlerCancel = cancel
    }

2. Keep cancelHandlerContext() as the only normal place that cancels an active handler context.

Expected cancel points:
- Close(ctx)
- onSessionClosed()
- failed Start(ctx) activation cleanup, if a context somehow exists

3. Update Start(ctx):
Replace:

    c.setHandlerContext()
    c.callbacksEnabled.Store(true)

with:

    c.ensureHandlerContext()
    c.callbacksEnabled.Store(true)

4. If setHandlerContext() is still useful for tests, either:
- update tests to call ensureHandlerContext(), or
- keep setHandlerContext() private but make Start(ctx) use ensureHandlerContext()

Preferred approach:
- Use ensureHandlerContext() everywhere a non-destructive context creation is needed.
- Use cancelHandlerContext() when lifecycle ends.
- Avoid helpers that cancel existing context unless their name clearly says so.

5. Keep handlerContext() behavior:
- return current handler context when available
- return context.Background() only as fallback when client has not started or context is unavailable

6. Do not change:
- RegisterConfigureHandler(...)
- RegisterActionHandler(...)
- RegisterResultHandler(...)
- RegisterStatusHandler(...)
- callback decode/validate behavior
- nil message guards
- registry behavior
- reconnect restore behavior
- subject builder behavior

Tests to add/update:
Add unit tests only.

Do not add integration tests for this small fix.

Required tests:

1. Repeated Start does not cancel active handler context

Test shape:
- create client
- stub startSessionFn to return nil
- stub activateAllSubscriptionsFn to return nil
- call Start(ctx)
- capture first handler context using client.handlerContext()
- assert first context is not canceled
- call Start(ctx) again
- capture second handler context
- assert first and second are the same context, or at minimum assert first context was not canceled
- assert callbacksEnabled remains true

Example:

    func TestRepeatedStartDoesNotCancelActiveHandlerContext(t *testing.T) {
        client, err := New(testConfig())
        if err != nil {
            t.Fatalf("New returned unexpected error: %v", err)
        }

        client.startSessionFn = func(context.Context) error { return nil }
        client.activateAllSubscriptionsFn = func(string) error { return nil }

        if err := client.Start(context.Background()); err != nil {
            t.Fatalf("first Start returned error: %v", err)
        }

        firstCtx := client.handlerContext()

        if err := client.Start(context.Background()); err != nil {
            t.Fatalf("second Start returned error: %v", err)
        }

        secondCtx := client.handlerContext()

        if firstCtx != secondCtx {
            t.Fatal("expected repeated Start to preserve active handler context")
        }

        select {
        case <-firstCtx.Done():
            t.Fatal("expected repeated Start not to cancel active handler context")
        default:
        }

        if !client.callbacksEnabled.Load() {
            t.Fatal("expected callbacksEnabled to remain true after repeated Start")
        }
    }

2. Start recreates handler context if previous one was canceled

Test shape:
- create client
- stub startSessionFn and activateAllSubscriptionsFn
- Start(ctx)
- capture first context
- call cancelHandlerContext()
- verify first context is canceled
- call Start(ctx)
- capture second context
- assert second context is not nil and not canceled
- assert second context is not the same canceled context

3. Close still cancels active handler context

If already covered, keep existing test.
If existing test depends on setHandlerContext(), update it to use Start(ctx) or ensureHandlerContext().

4. onSessionClosed still cancels active handler context

If already covered, keep existing test.
If existing test depends on setHandlerContext(), update it to use ensureHandlerContext() or Start(ctx).

5. Failed Start activation still does not leave an active handler context

Existing test should continue passing.
If needed, update assertion to check handlerContext fields safely.

Concurrency considerations:
- Protect handlerCtx and handlerCancel with Client.mu.
- Do not hold locks while invoking user handlers.
- Do not hold locks while calling cancel functions if avoidable.
- If cancelHandlerContext() currently copies cancel under lock, clears fields, unlocks, then calls cancel, keep that pattern.
- ensureHandlerContext() can create context under lock because it does not invoke user code and is cheap.

Build / verification:
- Run gofmt on changed Go files.
- Run go test ./...
- Run go test -race ./...
- If integration tests are not touched, do not run them unless required by the repo workflow.
- Do not claim tests passed unless they were actually run.

After coding, summarize:
1. Files changed
3. How repeated Start(ctx) is now handled
4. Tests added or updated
5. Commands run and results
6. Confirmation that public APIs did not change
7. Confirmation that subscription registry, reconnect restore, subject builder, and callback binding behavior were not redesigned
