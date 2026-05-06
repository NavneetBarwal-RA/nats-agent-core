Read PROMPTS/IMPLEMENTATION_PROMPT.md carefully.
Also read SPEC.md for additional design context.
Use PROMPTS/IMPLEMENTATION_PROMPT.md as the primary source of truth for implementation scope, architecture, runtime behavior, and boundaries.

Also read REQUIREMENTS_CHECKLIST.md and the current codebase carefully before making changes.

Build on the existing Phase 1, Phase 2, Phase 3, and Phase 4 code.
Treat the current public API as the baseline.
Preserve all existing exported structs, field names, JSON tags, and method signatures unless a compile-blocking issue requires a minimal compatibility fix.

Now implement only:
1. internal subscribe wrappers for configure/action/result/status receive paths
2. internal subscription registry used to remember subscription intent
3. callback binding and dispatch logic for registered handlers
4. the runtime wiring needed to back these existing or intended public APIs:
   - RegisterConfigureHandler(...)
   - RegisterActionHandler(...)
   - RegisterResultHandler(...)
   - RegisterStatusHandler(...)
5. reconnect-safe subscription restoration using the saved subscription registry
6. startup activation of handlers registered before Start(ctx)
7. receive-side result correlation by preserving and exposing rpc_id in received ResultEnvelope values

Requirements to focus on:
- NATS-LIB-04
- NATS-LIB-16
- NATS-LIB-17
- NATS-LIB-23
- NATS-LIB-25

Goal:
Add the real receive-side subscription and handler runtime layer so the existing public API becomes operational for:
- subscribing to configure notifications
- subscribing to action commands
- subscribing to result messages
- subscribing to status messages
- registering agent-provided handlers
- decoding and validating received envelopes before dispatch
- restoring subscriptions after reconnect
- preserving rpc_id for result correlation

Core implementation rules:
- Use only the Go standard library plus:
  - github.com/nats-io/nats.go
  - github.com/nats-io/nats.go/jetstream
- Keep the library workload-agnostic and agent-agnostic
- Keep the implementation modular, race-safe, and easy to review
- Protect mutable runtime state with mutexes where needed
- Avoid global variables
- Reuse the existing Phase 2 contract/codec/validation helpers where appropriate
- Reuse the existing Phase 3 subject helpers rather than rebuilding raw subject strings
- Reuse the existing Phase 4 session/runtime connection state rather than opening new NATS connections
- Keep handler registration thin and transport-focused
- Keep callback binding reusable instead of duplicating decode/validate/dispatch code in each handler path
- Do not replace existing public models with a different architecture
- Do not invent extra domain-specific APIs unless clearly necessary and minimal

Strict public API rule:
- Preserve all existing exported public structs, field names, JSON tags, and method signatures unless there is a compile-blocking issue that cannot be solved internally.
- If RegisterConfigureHandler, RegisterActionHandler, RegisterResultHandler, or RegisterStatusHandler already exist, wire them to real runtime behavior without changing their signatures.
- If any of these handler registration APIs do not exist yet, add them using the naming, style, context usage, and error-return style already used by the project.
- If a minimal public API fix is absolutely required, keep it as small as possible and report it clearly at the end.

Implementation scope boundaries:
- Do not implement test files in this phase
- Do not add unit tests in this phase
- Do not add integration tests in this phase
- Do not add examples or README work in this phase unless a tiny comment/doc correction is required
- Do not implement host reboot logic
- Do not implement host script execution
- Do not implement VyOS config translation
- Do not implement VyOS command execution
- Do not implement trace/rtty execution
- Do not implement local apply/rollback logic
- Do not implement cloud-side policy validation
- Do not implement durable JetStream stream/consumer action queues
- Do not redesign the desired-config KV storage model from Phase 4
- Do not redesign Start(ctx), Close(ctx), Health(), StoreDesiredConfig(...), LoadDesiredConfig(...), WatchDesiredConfig(...), or StartupReconcile(...)
- Do not implement a large pending-RPC correlation subsystem unless a small existing hook already exists and only needs wiring
- Do not make receive-side result correlation depend on external agent state

Subject expectations:
Use the existing centralized subject helpers from the earlier subject/routing phase.

The receive-side subjects must follow the standard target-first model:
- configure notification subject:
  - cmd.configure.<target>
- action command subject:
  - cmd.action.<target>.<action>
- result subject:
  - result.<target>
- status subject:
  - status.<target>

Subject behavior:
- Do not scatter raw subject strings across the codebase
- Do not introduce action-first routing
- Validate target before building any target-scoped subject
- Validate action before building any action-scoped subject
- Return clear typed validation errors for malformed target/action values
- Keep subject generation centralized and easy to audit

Subscribe wrapper expectations:
Implement common internal subscribe wrappers for:
- configure notification subscriptions
- action command subscriptions
- result subscriptions
- status subscriptions

Each subscribe wrapper should:
1. validate the routing input
2. build the subject using existing subject helpers
3. obtain the active NATS connection from the existing client/session runtime
4. subscribe using the existing NATS connection
5. support queue group subscriptions if the current config/options model already exposes queue group support
6. return an active subscription handle that can be unsubscribed or drained during cleanup
7. return typed errors on validation, disconnected/not-started, or subscribe failure
8. use existing logger hooks where available
9. avoid exposing raw NATS subscription details through public APIs

Subscribe wrappers must stay transport-focused.
They must not execute workload logic.

Handler registration expectations:
Wire the public handler registration APIs to real runtime behavior.

RegisterConfigureHandler(...) should:
1. validate target and handler input
2. build or record the configure subject for cmd.configure.<target>
3. create a subscription intent for configure receive handling
4. store that intent in the subscription registry
5. activate the subscription immediately if the client is already started and connected
6. defer activation until Start(ctx) if the client is not started yet

RegisterActionHandler(...) should:
1. validate target, action, and handler input
2. build or record the action subject for cmd.action.<target>.<action>
3. create a subscription intent for action receive handling
4. store that intent in the subscription registry
5. activate the subscription immediately if the client is already started and connected
6. defer activation until Start(ctx) if the client is not started yet

RegisterResultHandler(...) should:
1. validate target and handler input
2. build or record the result subject for result.<target>
3. create a subscription intent for result receive handling
4. store that intent in the subscription registry
5. activate the subscription immediately if the client is already started and connected
6. defer activation until Start(ctx) if the client is not started yet

RegisterStatusHandler(...) should:
1. validate target and handler input
2. build or record the status subject for status.<target>
3. create a subscription intent for status receive handling
4. store that intent in the subscription registry
5. activate the subscription immediately if the client is already started and connected
6. defer activation until Start(ctx) if the client is not started yet

Registration behavior:
- Registration before Start(ctx) must be supported
- Registration after Start(ctx) must be supported
- Repeated registration for the same subject/handler kind should not create duplicate active subscriptions accidentally
- If duplicate registration is allowed by current project style, it must still create distinct registry entries intentionally and safely
- If duplicate registration is not allowed, return a clear typed error
- Nil handlers must return a clear typed validation error
- Handler registration must not require the consuming agent to access raw NATS APIs

Callback binding expectations:
Implement reusable callback binding helpers.

The callback binding layer must sit between raw NATS messages and agent-provided handlers.

For each received NATS message, the callback binding should:
1. read the NATS message payload
2. decode JSON using existing contract/codec helpers
3. validate the decoded envelope using shared transport-level validation
4. preserve all correlation fields exactly as received
5. call the registered user handler with the appropriate typed public message
6. handle malformed payloads safely
7. avoid panic
8. log decode/validation/handler errors if logger hooks already exist
9. avoid holding internal locks while invoking user handlers

Callback binding must not add workload-specific validation.

Valid shared validation examples:
- required version if the current model requires it
- required rpc_id where the current envelope requires it
- required target
- required action for action commands
- required uuid/config identity where configure models require it
- basic payload presence only where the shared model requires it

Invalid validation examples:
- do not validate VyOS command syntax
- do not validate host script content
- do not validate reboot policy
- do not validate trace-specific fields
- do not validate cloud business policy

Expected handler message behavior:
Use existing public models where possible.

Configure handler:
- should receive the existing configure notification or configure receive model
- must preserve target
- must preserve rpc_id if present in the model
- must preserve uuid/config identity if present in the model

Action handler:
- should receive the existing action command or action receive model
- must preserve target
- must preserve action
- must preserve rpc_id if present in the model
- must preserve payload as the existing model defines it

Result handler:
- should receive the existing ResultEnvelope or result receive model
- must preserve target
- must preserve rpc_id exactly as received
- must expose rpc_id clearly through the public model passed to the handler
- must not generate a new rpc_id
- must not overwrite rpc_id
- must not hide rpc_id only in internal metadata

Status handler:
- should receive the existing StatusEnvelope or status receive model
- must preserve target
- must preserve rpc_id if present in the model
- must preserve status fields as the existing model defines them

Subscription registry expectations:
Implement or complete an in-memory subscription registry.

The registry must store subscription intent, not only active *nats.Subscription handles.

The registry must contain enough information to recreate all subscriptions after reconnect.

Each registry entry should track:
- stable internal registration ID or key
- subscription kind:
  - configure
  - action
  - result
  - status
- target
- action when applicable
- generated subject
- queue group when applicable
- callback binder or bound NATS callback factory
- active NATS subscription handle when currently active
- active/inactive state
- last activation error if useful for health/log visibility
- enough metadata to avoid duplicate active subscriptions during restore

Registry behavior:
- Must be concurrency-safe
- Must allow registration before Start(ctx)
- Must allow activation after Start(ctx)
- Must allow restoring all saved registrations after reconnect
- Must avoid duplicate active subscriptions during Start(ctx) and reconnect restore
- Must support cleanup of active subscriptions during Close(ctx)
- Must not call user handlers while holding registry locks
- Must not hold registry locks during long-running NATS operations if avoidable
- Must keep registry internals unexported unless the current public API already exposes a registration handle concept

Suggested registry operations:
- Add(...)
- List(...)
- Activate(...)
- ActivateAll(...)
- RestoreAll(...)
- Deactivate(...)
- DeactivateAll(...)
- CloseAll(...)

Use names that match the current codebase style.

Start(ctx) integration expectations:
During Start(ctx), after the NATS connection is established and runtime session state is ready:
1. activate all handler registrations already saved in the subscription registry
2. subscribe using the active NATS connection
3. update registry entries with active subscription handles
4. leave existing Phase 4 KV/session startup behavior intact
5. leave StartupReconcile(...) behavior intact
6. expose subscription activation failures through typed errors and/or existing health/logger hooks

This sequence must work:

client := New(...)
client.RegisterConfigureHandler(...)
client.RegisterActionHandler(...)
client.RegisterResultHandler(...)
client.RegisterStatusHandler(...)
client.Start(ctx)

After Start(ctx), all registered handlers should be active.

Registration after Start(ctx) expectations:
If a handler is registered after the client is already started and connected:
1. create and store the registry intent
2. immediately activate the subscription using the active session
3. update the registry entry with the active subscription handle
4. return a subscribe failure if immediate activation fails

Reconnect expectations:
NATS-LIB-04 requires the library to restore required subscriptions after reconnect.

Use the subscription registry as the source of truth.

On reconnect or reconnect recovery:
1. keep all saved subscription intents
2. treat old active subscription handles as stale
3. recreate active subscriptions from registry entries
4. update each registry entry with the new active subscription handle
5. avoid duplicate active subscriptions
6. report restore failures through existing logger/health/error mechanisms where available
7. do not require consuming agents to manually re-register handlers

If Phase 4 already has reconnect callbacks or recovery hooks, wire registry restore into that existing path.

If Phase 4 does not expose a clean reconnect hook, add the smallest internal hook needed to invoke registry restoration after reconnect.

Do not redesign the session layer.

Do not open a separate connection for restoring subscriptions.

Do not make reconnect restoration a public responsibility.

Close(ctx) expectations:
Close(ctx) should clean up active subscriptions owned by the client.

Close behavior should:
1. transition existing health/session state according to Phase 4 behavior
2. deactivate or unsubscribe all active registry subscriptions
3. clear stale active subscription handles
4. avoid invoking user handlers after close where practical
5. avoid goroutine leaks
6. then continue the existing Phase 4 drain/close behavior for the NATS connection

If the existing client lifecycle supports restarting after Close(ctx), preserve subscription intent so Start(ctx) can activate registrations again.

If the existing client lifecycle treats Close(ctx) as terminal, cleanup should still leave registry state consistent and inactive.

Follow the lifecycle semantics already established in the codebase.

Receive-side rpc_id correlation expectations:
NATS-LIB-23 requires received results to correlate through rpc_id.

Result receive behavior:
- Decode the incoming result message into the existing ResultEnvelope or result receive model
- Preserve rpc_id exactly as received
- Pass the decoded result to the registered result handler with rpc_id visible on the public model
- Do not generate a new rpc_id
- Do not overwrite rpc_id
- Do not drop rpc_id during callback binding
- Do not hide rpc_id only in logs or internal metadata

Configure/action/status receive behavior:
- Preserve rpc_id where the model contains it
- Preserve uuid/config identity for configure notifications where the model contains it
- Preserve target and action fields as defined by the public models

This phase should not build a large request/response waiting system.
The minimum required behavior is that received results expose rpc_id clearly to the registered handler.

Health / recovery visibility expectations:
If the existing health model has fields suitable for subscription or recovery state, update them minimally.

At minimum:
- reconnecting/degraded state from Phase 4 must not be broken
- subscription restore failures should be visible through last error/log hooks where available
- subscription restore should not silently fail if it prevents handlers from working

Do not redesign the health model in this phase.

Logging expectations:
If logger hooks already exist, use them around:
- handler registration
- subscribe activation success/failure
- callback decode failure
- callback validation failure
- user handler callback failure
- reconnect restore start/success/failure
- subscription cleanup

Do not create a new logging framework.

Metrics expectations:
If metrics hooks already exist and have obvious counters/timers for messaging operations, update them minimally.

Possible useful metric events:
- subscription registered
- subscription activated
- subscription restored
- subscription failed
- message received
- message decode failed
- handler failed

Do not create a broad observability redesign in this phase.

Concurrency expectations:
The implementation must be safe for:
- registering handlers before Start(ctx)
- registering handlers after Start(ctx)
- reconnect restoration while registry entries exist
- Close(ctx) while subscriptions exist
- receive callbacks running concurrently
- Health() being called while subscriptions are activating/restoring

Concurrency rules:
- Protect mutable client and registry state
- Do not hold locks while invoking user callbacks
- Do not hold locks across long-running NATS operations unless the existing session design requires it
- Avoid deadlocks between client locks, session locks, and registry locks
- Avoid double-subscribe during concurrent Start(ctx), Register*Handler(...), and reconnect restore paths
- Keep callback dispatch independent from registry mutation

Typed error expectations:
Use the existing public typed error model and existing error codes consistently.

Map failures as follows, using the closest existing codes if exact names differ:
- nil handler -> CodeValidationFailed or closest validation code
- invalid target/action -> CodeValidationFailed or closest validation code
- client not started / disconnected -> CodeDisconnected
- client closed -> CodeDisconnected or closest lifecycle/state code
- subscribe failure -> CodeSubscribeFailed if available, otherwise closest NATS/connection operation failure code
- callback decode failure -> log and drop message; do not call user handler
- callback validation failure -> log and drop message; do not call user handler
- reconnect restore failure -> CodeReconnectFailed or closest connection/recovery code
- cleanup/unsubscribe failure -> CodeShutdown or closest lifecycle code

General error rules:
- Keep retryability metadata sensible and consistent
- Do not replace typed errors with plain formatted errors
- Wrap underlying causes where useful
- Prefer clear typed failures over vague generic errors
- Do not panic for normal input/runtime failures

Suggested files:
- internal/transport/subscribe.go
- internal/transport/handlers.go
- internal/transport/callbacks.go
- internal/registry/registry.go
- internal/registry/entry.go
- internal/registry/restore.go

Potential integration touch points:
- agentcore/client.go
- agentcore/handlers.go if present
- agentcore/config.go only if a minimal handler/queue option correction is truly required
- agentcore/models.go only if a minimal receive model correction is truly required
- internal/session/session.go only if a small reconnect-restore hook is required
- internal/session/callbacks.go only if reconnect callback wiring belongs there
- internal/subjects only if a small missing subject helper is required
- internal/contract only if a small decode/helper addition is required

Add more small focused files only if clearly needed.

Build / verification requirements:
- Run gofmt on all changed Go files
- Run go test ./...
- Do not add new test files in this phase
- If existing go test ./... fails because of pre-existing issues or because later test phases are not implemented yet, report that clearly instead of masking it
- Do not silently leave the repository in a non-compiling state

After coding:
- summarize exactly which files were added or changed
- explain how the implementation aligns with PROMPTS/IMPLEMENTATION_PROMPT.md
- map each added/changed file to these requirement IDs:
  - NATS-LIB-04
  - NATS-LIB-16
  - NATS-LIB-17
  - NATS-LIB-23
  - NATS-LIB-25
- list any minimal public API adjustments made, if any
- list what is intentionally deferred to later phases
- explicitly confirm that no tests were added in this phase
- explicitly confirm that no workload/business logic was added

Review checklist for this phase:
- common subscribe wrappers exist for configure/action/result/status
- subscribe wrappers use centralized subject helpers
- raw subject strings are not scattered through the codebase
- handler registration APIs are backed by real runtime behavior
- handler registration before Start(ctx) is supported
- handler registration after Start(ctx) is supported
- callback binding is reusable and shared
- callback binding decodes received envelopes through shared codec helpers
- callback binding validates only shared transport-level envelope fields
- malformed messages do not panic
- malformed messages do not invoke user handlers
- user handlers receive typed public message models
- result handlers receive rpc_id unchanged and clearly visible
- configure receive path preserves uuid/config identity where applicable
- action receive path preserves target/action/rpc_id where applicable
- status receive path preserves target/rpc_id where applicable
- subscription registry exists
- registry stores subscription intent, not only active NATS handles
- registry is concurrency-safe
- Start(ctx) activates pre-registered handlers
- reconnect restoration uses saved registry intent
- reconnect restoration avoids duplicate active subscriptions
- Close(ctx) cleans up active subscriptions
- existing Phase 4 session/KV/StartupReconcile behavior is not broken
- no tests were added
- no workload/business logic was added
