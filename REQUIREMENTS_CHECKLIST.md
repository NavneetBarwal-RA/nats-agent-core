# NATS Agent Core - Requirements Checklist

Use this checklist while generating and reviewing the library phase by phase.

How to use:
- Mark `[x]` only when you can trace the requirement in:
  - public API
  - implementation
  - tests
- Add file names, commit IDs, or short notes in the **Review Notes** column
- Do not mark a requirement complete just because Codex mentioned it; verify it in code

---

## Review status legend

- `[ ]` Not reviewed yet
- `[~]` Partially covered / needs more review
- `[x]` Reviewed and acceptable
- `[!]` Problem found / redesign needed

---

## Functional Requirements

| Status | Requirement ID | Requirement | What this means in simple words | Expected proof in code | Expected proof in tests | Review Notes |
|---|---|---|---|---|---|---|
| [ ] | NATS-LIB-01 | Standard NATS connection | Library can connect to configured NATS servers | `New(...)`, `Start(...)`/`Connect(...)`, NATS config usage in `client.go` or session layer | Integration test connects to real NATS | |
| [ ] | NATS-LIB-02 | Full client session lifecycle | Library supports startup, running state, and clean shutdown | `Start(ctx)`, `Close(ctx)`, drain/cleanup logic | Tests for start/stop and shutdown behavior | |
| [ ] | NATS-LIB-03 | Reconnect on connection loss | Library reconnects automatically after temporary disconnect | reconnect options, callbacks, retry policy | Integration or targeted session test | |
| [ ] | NATS-LIB-04 | Restore subscriptions after reconnect | Library remembers subscriptions and restores them after reconnect | in-memory subscription registry + restore logic | Test that handlers still work after reconnect | |
| [ ] | NATS-LIB-05 | Create/manage JetStream and KV context | Library creates JetStream handle and binds/creates KV bucket | `jetstream.New(nc)`, KV setup in session/KV layer | Integration test with `nats-server -js` | |
| [ ] | NATS-LIB-06 | Expose session health state | Library reports connection/session health in a read-only way | `Health()` + `HealthSnapshot` | Unit test for health state changes | |
| [ ] | NATS-LIB-07 | Expose failures clearly to caller | Public methods return useful errors instead of hiding failures | typed errors returned from public APIs | Unit tests for failure cases | |
| [ ] | NATS-LIB-08 | Standard message contract | All agents use the same message structs/envelopes | public types in `models.go` or contract layer | Unit tests for model usage/consistency | |
| [ ] | NATS-LIB-09 | JSON codec for standard envelopes | Library consistently encodes/decodes messages as JSON | marshal/unmarshal helpers, JSON tags | Unit tests for encode/decode | |
| [ ] | NATS-LIB-10 | Common correlation fields | `rpc_id`, `uuid`, etc. are preserved across flows | fields in models and pass-through in publish/handle flow | Tests that correlation fields survive round-trip | |
| [ ] | NATS-LIB-11 | Configure message model | Library has a clear configure request model | `ConfigureCommand`, `ConfigureNotification`, related structs | Unit tests for configure validation/codec | |
| [ ] | NATS-LIB-12 | Action message model | Library has a clear action request model | `ActionCommand` + subject building | Unit tests for action validation/codec | |
| [ ] | NATS-LIB-13 | Result and status message model | Result and status are separately represented | `ResultEnvelope`, `StatusEnvelope` | Unit tests for both message types | |
| [ ] | NATS-LIB-14 | Envelope validation | Library checks required fields and message sanity | validation helpers in contract layer | Unit tests for invalid/missing fields | |
| [ ] | NATS-LIB-15 | Submit configure through public API | Agent can call a public method to submit configure | `SubmitConfigure(...)` | Integration/unit test for configure submit flow | |
| [ ] | NATS-LIB-16 | Submit action through public API | Agent can call a public method to submit action | `SubmitAction(...)` | Integration/unit test for action submit flow | |
| [ ] | NATS-LIB-17 | Publish result/status through public API | Agent can publish result and status using shared API | `PublishResult(...)`, `PublishStatus(...)` | Tests for result/status publication | |
| [ ] | NATS-LIB-18 | Desired config store/retrieve API | Library can store and load desired config from KV | `StoreDesiredConfig(...)`, `LoadDesiredConfig(...)` | KV-related unit/integration tests | |
| [ ] | NATS-LIB-19 | Expose config identity of desired config | Caller can know the UUID of the currently stored desired config | `LoadDesiredConfig(...)` returns a record containing config UUID | Unit tests for UUID exposure from desired config record | |
| [ ] | NATS-LIB-20 | Standard subject generation | Library creates subject names in one standard way | subject helpers/builders | Unit tests for subject generation | |
| [ ] | NATS-LIB-21 | Desired config load on startup/recovery | Agent can reload latest desired config on startup/recovery | `StartupReconcile(...)` and/or load helpers | Integration or unit tests for recovery path | |
| [ ] | NATS-LIB-22 | Publish action to owner target | Actions are routed to correct target-specific subject | target-based action subject building | Unit tests for routing to correct subject | |
| [ ] | NATS-LIB-23 | Subject validation / safe routing | Library rejects malformed subject inputs | subject validation helpers | Unit tests for invalid target/action input | |
| [ ] | NATS-LIB-24 | Optional watch for desired config changes | Library can optionally watch config changes in KV | `WatchDesiredConfig(...)` | Unit/integration test for watch callback | |
| [ ] | NATS-LIB-25 | Startup reconciliation / reload latest desired state | Library helps agent recover latest desired state after restart/reconnect | `StartupReconcile(...)` or equivalent flow | Recovery-focused tests | |
| [ ] | NATS-LIB-26 | Typed errors and clear failure handling | Errors have structured codes and retryability info | typed `Error`, `Code` enum, retryability metadata | Unit tests for error type behavior | |
| [ ] | NATS-LIB-27 | Health state model | Health is represented as a proper structured model | `HealthSnapshot` and related internal state | Unit tests for health model fields | |
| [ ] | NATS-LIB-28 | Health snapshot / health exposure | Health API returns safe read-only state | `Health()` returns snapshot/copy | Unit tests for snapshot safety | |
| [ ] | NATS-LIB-29 | Logger hooks | Agent can plug in its own logger | `Logger` interface and injection path | Unit tests or usage examples | |
| [ ] | NATS-LIB-30 | Metrics hooks | Agent can plug in metrics collection | `Metrics` interface and hook points | Unit tests or usage examples | |
| [ ] | NATS-LIB-31 | Configure outcome carries config identity | Configure result/status includes the config UUID that was attempted or applied | `ResultEnvelope`/configure-specific result model includes config UUID | Unit/integration tests for configure result carrying UUID | |

---

## Non-Requirements / Out of Scope

These are also important during review.  
If you see these implemented inside the shared library, that is a design violation.

| Status | Requirement ID | Non-requirement | What this means in simple words | What should NOT exist in shared library code | Review Notes |
|---|---|---|---|---|---|
| [ ] | NATS-LIB-NR-01 | No workload-specific config translation in library | Shared library should not translate generic config into VyOS/host-specific commands | no VyOS CLI translation, no host-specific config conversion | |
| [ ] | NATS-LIB-NR-02 | No reboot/script/trace/rtty execution in library | Shared library should not perform actual business actions | no reboot calls, no script execution, no trace implementation | |
| [ ] | NATS-LIB-NR-03 | No local apply/rollback/state transition logic in library | Shared library should not own workload apply engines | no rollback engine, no workload state machine | |
| [ ] | NATS-LIB-NR-04 | No cloud-side business validation policy in library | Shared library should only do transport-level validation | no deep cloud policy validation logic | |
| [ ] | NATS-LIB-NR-05 | No revision-driven config contract | Shared library should not expose revision/history as part of the functional config model | no `LoadDesiredConfigRevision(...)` as required design API, no revision-based sync logic | |

---

## Phase-by-Phase Review Plan

### Phase 1 - Bootstrap and Public API
Goal:
- module setup
- public types
- public config
- client skeleton
- error/logger/metrics types

Main requirements to review:
- NATS-LIB-08
- NATS-LIB-09
- NATS-LIB-10
- NATS-LIB-11
- NATS-LIB-12
- NATS-LIB-13
- NATS-LIB-14
- NATS-LIB-26
- NATS-LIB-29
- NATS-LIB-30
- NATS-LIB-31

Checklist:
- [ ] Public types exist
- [ ] JSON tags are clean
- [ ] `context.Context` appears in networked public methods
- [ ] Error / logger / metrics types are present
- [ ] No internal business logic leaked into public models
- [ ] Configure result model includes config UUID

Commit note:
- `feat(api): bootstrap module and public API types`

---

### Phase 2 - Contract and Validation
Goal:
- envelope validation
- JSON codec
- transport-level sanity checks

Main requirements to review:
- NATS-LIB-08
- NATS-LIB-09
- NATS-LIB-10
- NATS-LIB-14
- NATS-LIB-31

Checklist:
- [ ] Validation checks required fields
- [ ] Validation is transport-level only
- [ ] Correlation fields preserved in structs
- [ ] Encode/decode helpers exist
- [ ] Configure result/status preserves config UUID

Commit note:
- `feat(contract): add envelope validation and JSON codec`

---

### Phase 3 - Subjects and Routing
Goal:
- central subject helpers
- safe routing validation

Main requirements to review:
- NATS-LIB-12
- NATS-LIB-20
- NATS-LIB-22
- NATS-LIB-23

Checklist:
- [ ] Configure subject helper exists
- [ ] Action subject helper exists
- [ ] Result/status subject helpers exist
- [ ] Invalid target/action input is rejected
- [ ] Raw subject strings are not scattered across the codebase

Commit note:
- `feat(subjects): add subject builders and routing validation`

---

### Phase 4 - Session, JetStream, KV, Health
Goal:
- NATS session
- JetStream handle
- KV bucket
- health exposure
- graceful shutdown

Main requirements to review:
- NATS-LIB-01
- NATS-LIB-02
- NATS-LIB-03
- NATS-LIB-05
- NATS-LIB-06
- NATS-LIB-18
- NATS-LIB-19
- NATS-LIB-21
- NATS-LIB-25
- NATS-LIB-27
- NATS-LIB-28

Checklist:
- [ ] `jetstream.New(nc)` is used
- [ ] KV bind/create exists
- [ ] reconnect callbacks exist
- [ ] health snapshot exists
- [ ] shutdown drains the connection
- [ ] mutable shared state is protected safely
- [ ] KV usage is aligned to latest-state config storage

Commit note:
- `feat(session): add NATS session, JetStream, KV, and health`

---

### Phase 5 - Submission APIs and Handlers
Goal:
- configure/action submit APIs
- result/status publish APIs
- desired config store/load/watch APIs
- handler registration
- subscription registry
- reconnect restore

Main requirements to review:
- NATS-LIB-04
- NATS-LIB-15
- NATS-LIB-16
- NATS-LIB-17
- NATS-LIB-18
- NATS-LIB-19
- NATS-LIB-24
- NATS-LIB-25
- NATS-LIB-31

Checklist:
- [ ] `SubmitConfigure(...)` exists
- [ ] `SubmitAction(...)` exists
- [ ] `PublishResult(...)` exists
- [ ] `PublishStatus(...)` exists
- [ ] `StoreDesiredConfig(...)` exists
- [ ] `LoadDesiredConfig(...)` exists
- [ ] `WatchDesiredConfig(...)` exists
- [ ] `StartupReconcile(...)` exists
- [ ] handler registration methods exist
- [ ] subscription registry exists
- [ ] reconnect restore uses registry
- [ ] configure flow is store-then-notify
- [ ] action flow is direct publish
- [ ] configure outcome includes config UUID

Commit note:
- `feat(transport): add submission APIs, handlers, and reconnect restoration`

---

### Phase 6 - Errors, Logging, Metrics, Observability
Goal:
- finalize typed errors
- logger hooks
- metrics hooks
- observability consistency

Main requirements to review:
- NATS-LIB-07
- NATS-LIB-26
- NATS-LIB-27
- NATS-LIB-28
- NATS-LIB-29
- NATS-LIB-30

Checklist:
- [ ] typed error model is used by public APIs
- [ ] retryability metadata exists
- [ ] logger hook is injectable
- [ ] metrics hook is injectable
- [ ] no global observability state
- [ ] health remains read-only

Commit note:
- `feat(observe): add typed errors, logger hooks, and metrics hooks`

---

### Phase 7 - Unit Tests
Goal:
- prove main behavior in isolated tests

Checklist:
- [ ] envelope validation tests
- [ ] subject generation tests
- [ ] error model tests
- [ ] health model tests
- [ ] submission logic tests
- [ ] registry/handler tests
- [ ] config UUID propagation tests for configure flows

Commit note:
- `test(unit): add unit coverage for core library behavior`

---

### Phase 8 - Integration Tests
Goal:
- prove real behavior against actual NATS + JetStream

Checklist:
- [ ] real `nats-server -js` test setup
- [ ] connect/start/close tested
- [ ] configure flow tested
- [ ] action flow tested
- [ ] result/status flow tested
- [ ] reconnect behavior tested if practical
- [ ] configure result UUID contract tested

Commit note:
- `test(integration): add real NATS JetStream integration coverage`

---

### Phase 9 - Examples and README
Goal:
- make library usage easy to understand

Checklist:
- [ ] command-agent example
- [ ] host-agent example
- [ ] vyos-agent example
- [ ] quick-start README
- [ ] examples show library usage, not business logic internals
- [ ] examples reflect UUID-based latest-state config model

Commit note:
- `docs(examples): add examples and quickstart README`

---

## Quick File Review Map

| File / Area | What to check |
|---|---|
| `agentcore/config.go` | public config structure is clear and minimal |
| `agentcore/models.go` | standard envelope and message types |
| `agentcore/client.go` | public client lifecycle and API surface |
| `internal/contract` | codec and validation |
| `internal/subjects` | subject generation and routing validation |
| `internal/session` | NATS connection, reconnect, lifecycle |
| `internal/kv` | desired config storage/retrieval/watch |
| `internal/transport` | submit/publish behavior |
| `internal/registry` | handler/subscription registry |
| `internal/observe` | logging and metrics hooks |
| `internal/errors` | typed errors and codes |
| `tests` | requirement proof through behavior |

---

## Final Review Questions

Before calling the library acceptable, answer these:

- [ ] Can I trace each public requirement to API + implementation + tests?
- [ ] Is configure clearly implemented as **store in KV, then notify**?
- [ ] Is action clearly implemented as **direct publish to target subject**?
- [ ] Is KV clearly used as a **single latest desired-config slot**?
- [ ] Is config sync clearly based on **UUID equality**, not KV revision ordering?
- [ ] Do configure outcomes include the config UUID that was attempted/applied?
- [ ] Are subjects centralized and consistent?
- [ ] Are envelopes standardized and validated?
- [ ] Is reconnect and subscription restoration handled?
- [ ] Is health exposed safely?
- [ ] Are typed errors, logger hooks, and metrics hooks present?
- [ ] Is business logic kept out of the shared library?
- [ ] Are examples and README understandable for future developers?

---

## Notes

Use this file as a living review document.

Add:
- commit hashes
- file names
- gaps found
- redesign notes
- questions for follow-up review
