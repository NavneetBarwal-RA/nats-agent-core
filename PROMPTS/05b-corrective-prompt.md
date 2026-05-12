Read the current Phase 5 implementation carefully.

Focus only on correcting the Phase 5 handler registration, subscription activation, and reconnect/cleanup semantics.

Do not add new feature scope.
Do not add integration tests.
Do not rewrite the overall architecture.
Do not change public API signatures unless absolutely required for compilation.

Context:
Phase 5 added:
- RegisterConfigureHandler(...)
- RegisterActionHandler(...)
- RegisterResultHandler(...)
- RegisterStatusHandler(...)
- subscription registry
- callback binding
- reconnect restore hook
- subscription health counters

The current implementation has two correctness issues that must be fixed:

1. Failed immediate activation leaves registry side effects

Current behavior:
- Register*Handler validates input
- adds the registration intent to the registry
- then calls activateAfterRegistration(...)
- if activation fails, Register*Handler returns an error
- but the registry entry remains stored

This is incorrect for the current public API because callers see registration failure but the handler remains registered internally.

Required behavior:
- If handler registration occurs before Start(ctx), store the registry intent and return nil.
- If handler registration occurs after Start(ctx), add the registry intent and immediately activate it.
- If immediate activation fails, rollback the newly added registry intent so the failed registration does not remain saved.
- After rollback, health subscription counters must be synced.
- Retrying the same registration after a failed immediate activation must not hit CodeRegistryConflict from the previous failed attempt.
- A handler that failed immediate registration must not later activate during reconnect or a later Start(ctx).

Implementation guidance:
- Add a registry removal method if one does not already exist.
- Removal should:
  - be concurrency-safe
  - remove the entry by ID
  - remove the corresponding duplicate-detection key
  - unsubscribe or return the active handle if the entry somehow became active
  - leave registry counts accurate
- Keep removal internal; do not expose a public unregister API in this corrective patch.
- Use the existing typed error model.
- Preserve successful deferred registration behavior before Start(ctx).

2. Start(ctx) enables callbacks before activation succeeds

Current behavior:
- Start(ctx) calls session.Start(ctx)
- then sets callbacksEnabled=true
- then activates all registered subscriptions
- if activation fails, Start(ctx) returns an error while callbacks may remain enabled and any partially activated subscriptions may still dispatch

This is incorrect because a failed Start(ctx) should not leave receive callbacks active.

Required behavior:
- Start(ctx) should call session.Start(ctx)
- then activate all pre-registered subscriptions
- only after all activation succeeds should callbacksEnabled be set to true
- if activation fails:
  - callbacksEnabled must remain false
  - any subscriptions activated during that failed Start(ctx) attempt must be deactivated/cleaned up
  - health subscription counters must be updated
  - Start(ctx) should return the activation error
- Do not break existing Phase 4 session/KV startup behavior.
- Do not close the NATS session automatically unless the existing lifecycle semantics already require that.
- The key requirement is no active receive callbacks after a failed subscription activation during Start(ctx).

Detailed behavior expectations:

A. Registration before Start(ctx)
- Valid registration should store intent only.
- It should not try to access the NATS connection.
- It should return nil.
- RegisteredSubscriptions should increase.
- ActiveSubscriptions should remain zero.

B. Registration after successful Start(ctx)
- Valid registration should store intent.
- It should immediately activate the subscription.
- If activation succeeds:
  - return nil
  - RegisteredSubscriptions should include the new registration
  - ActiveSubscriptions should include the new active subscription
- If activation fails:
  - rollback the newly added intent
  - cleanup any active handle created for that intent
  - return the activation error
  - RegisteredSubscriptions and ActiveSubscriptions should not include the failed registration

C. Duplicate registration
- Existing duplicate policy can stay as-is.
- Duplicate valid registration should still return CodeRegistryConflict if that is current behavior.
- A failed immediate activation must not create a stale duplicate conflict for a later retry.

D. Start(ctx) activation failure
- If one or more deferred subscriptions fail to activate during Start(ctx):
  - return the activation error
  - callbacksEnabled must be false
  - any subscriptions activated during that Start(ctx) activation pass must be unsubscribed/cleared
  - registry entries may remain as deferred intent for later retry, but they must not be marked active
  - ActiveSubscriptions should be zero after cleanup
- Preserve RegisteredSubscriptions because pre-Start deferred intent was valid and should remain registered for a future retry.

E. Reconnect restore behavior
- Reconnect restore should continue using saved registry intent.
- It should still avoid duplicate active subscriptions.
- It should not be broken by the new rollback/removal behavior.
- callbacksEnabled should still gate callback dispatch.
- If restore fails, keep existing logging/error-sink behavior.

F. Close(ctx)
- Close(ctx) should continue disabling callbacks first.
- Close(ctx) should continue cleaning up active subscriptions.
- Registry state should remain consistent after Close(ctx).
- Do not introduce goroutine leaks or double-unsubscribe panics.

Suggested implementation changes:
- Add Registry.Remove(id string) or Registry.RemoveAndDetach(id string) as appropriate.
- Consider returning an ActiveHandle from removal if cleanup is needed.
- Add a Client helper such as rollbackRegistration(id, op string) or removeRegisteredSubscription(id, op string).
- Update activateAfterRegistration(...) so failure after immediate activation attempts triggers rollback for the newly added entry.
- Update Start(ctx) so callbacksEnabled is set only after activateAllRegisteredSubscriptions(...) succeeds.
- Update activateAllRegisteredSubscriptions(...) or Start(ctx) to cleanup partial activations when activation fails.
- Be careful not to remove valid deferred registry entries during Start(ctx) failure; only clear active handles from the failed activation pass.
- Keep locks safe:
  - do not hold locks while invoking user handlers
  - avoid holding registry locks while calling NATS Unsubscribe where practical
  - avoid deadlocks between Client.subMu, Client.mu, registry locks, and session locks

Tests to update/add:
- Unit tests only.
- Do not add real NATS integration tests.
- Do not start nats-server.
- Do not require Docker.

Add or update tests for:

1. Registration rollback after immediate activation failure
- Simulate a started/degraded/disconnected state where RegisterResultHandler attempts immediate activation and fails.
- Assert the call returns CodeDisconnected or the existing activation error.
- Assert the newly attempted registration is not left in the registry.
- Assert a second retry does not fail with CodeRegistryConflict caused by stale intent.

2. Start activation failure cleanup
- Arrange deferred registry entries before Start(ctx).
- Use existing test seams/mocks if present to force subscription activation failure after session start or during activation.
- Assert Start(ctx) returns error.
- Assert callbacksEnabled is false.
- Assert ActiveSubscriptions is zero.
- Assert RegisteredSubscriptions still reflects deferred intent if the intent was valid.
- Assert no active subscription handles remain.

3. Successful Start still enables callbacks
- Confirm that when activation succeeds, callbacksEnabled is true.
- Confirm active subscription counts are correct.

4. Existing registration-before-Start behavior remains valid
- Valid Register*Handler before Start(ctx) still returns nil.
- RegisteredSubscriptions increments.
- ActiveSubscriptions remains zero.

5. Close behavior remains safe
- Close(ctx) disables callbacks.
- Close(ctx) clears active handles.
- Repeated Close remains safe if already covered; extend only if needed.

Do not over-refactor tests.
Prefer package-local tests if they already exist.
Use existing helper style:
- requireErrorCode
- testConfig
- existing fake subscription/session helpers if available

Build / verification:
- Run gofmt on all changed Go files.
- Run go test ./...
- If go test ./... fails because of unrelated pre-existing issues, report that clearly.
- Do not leave the repository in a non-compiling state.

After coding, summarize:
1. files changed
2. exact fix for registration rollback
3. exact fix for Start(ctx) callback enabling / activation cleanup
4. tests added or updated
5. behavior preserved for deferred registration, reconnect restore, and Close(ctx)
6. confirmation that no public API signatures changed
7. confirmation that no integration tests or workload logic were added
