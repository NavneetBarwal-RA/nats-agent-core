Read PROMPTS/IMPLEMENTATION_PROMPT.md carefully.
Also read SPEC.md for additional design context.
Use PROMPTS/IMPLEMENTATION_PROMPT.md as the primary implementation source of truth for code generation scope, API shape, and wire-contract behavior.

Also read REQUIREMENTS_CHECKLIST.md and the current codebase carefully.

Build on the already-corrected Phase 1 and Phase 2 code.
Treat the current public types and method signatures as the baseline unless a minimal compatibility correction is absolutely required.

Now implement only:
1. internal/subjects
2. centralized subject generation helpers
3. subject validation helpers
4. shared publish wrappers for major message types
5. configure store-then-notify publish path
6. explicit shared helpers for result and status publication

Requirements to focus on:
- NATS-LIB-12
- NATS-LIB-15
- NATS-LIB-20
- NATS-LIB-21
- NATS-LIB-22

Goal:
Add the shared messaging layer for subject creation and publish paths so routing stays centralized, publish behavior is reusable, and configure follows the required durable store-then-notify flow.

Rules:
- Centralize all subject generation in one internal package
- Reject malformed target/action routing inputs early
- Do not scatter raw subject strings across the codebase
- Implement configure, action, result, and status subject helpers
- Subject helpers are shared routing helpers for both publish-side and subscribe-side use, even though subscribe wrapper APIs are deferred to a later phase
- Keep shared publish logic centralized, small, and reusable
- Configure must follow store-then-notify, not direct full-config publish only
- Result and status publication must go through explicit shared helpers
- Reuse Phase 2 contract/codec/validation helpers where appropriate
- Do not implement subscribe wrappers yet
- Do not implement handler registration yet
- Do not implement reconnect restoration yet
- Do not implement receive-side result correlation logic in this phase
- Do not add tests in this phase
- Keep code modular, testable, and race-safe
- Do not invent extra domain-specific routing patterns not present in PROMPTS/IMPLEMENTATION_PROMPT.md or REQUIREMENTS_CHECKLIST.md

Subject expectations:
- Configure subject: cmd.configure.%s
- Action subject: cmd.action.%s.%s
- Result subject: result.%s
- Status subject: status.%s

Validation expectations:
- Validate target values used in subject construction
- Validate action values used in action subject construction
- Reject malformed or empty routing identifiers where required
- Keep validation limited to shared routing/subject sanity only
- Do not embed workload/business meaning into subject validation

Publish path expectations:
- Configure flow must:
  1. validate the configure request
  2. store desired configuration through the shared durable storage path
  3. publish a lightweight configure notification only after storage succeeds
- Action flow must publish through a shared helper using centralized subject builders
- Result flow must publish through an explicit shared helper
- Status flow must publish through an explicit shared helper
- Publish helpers should avoid ad hoc raw subject construction and duplicated publish logic

Suggested files:
- internal/subjects/subjects.go
- internal/subjects/validate.go
- internal/transport/publish.go
- internal/transport/configure.go

Add more small focused files only if clearly needed.

After coding:
- summarize exactly which files were added or changed
- explain how the implementation aligns with PROMPTS/IMPLEMENTATION_PROMPT.md
- map each added/changed file to the relevant requirement IDs from REQUIREMENTS_CHECKLIST.md, especially:
  - NATS-LIB-12
  - NATS-LIB-15
  - NATS-LIB-20
  - NATS-LIB-21
  - NATS-LIB-22
- list any minimal public API adjustments made, if any
- list what is intentionally deferred to later phases

Review checklist for this phase:
- configure, action, result, and status subject helpers exist
- malformed target/action inputs are rejected
- one place for subject creation exists
- subject helpers are reusable by future subscribe-side code as well as current publish-side code
- common publish wrappers exist for major message types
- configure follows store-then-notify
- result and status use explicit shared publish helpers
- raw subject strings are not scattered through the codebase
- no subscribe/handler/reconnect logic leaked into this phase
- no workload/business logic was added
