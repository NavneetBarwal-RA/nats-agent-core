Read PROMPTS/TESTS_STANDARD_TEMPLATE.md first and follow it exactly for:
- structure
- wording
- formatting
- naming
- comment style

Also read:
- SPEC.md
- REQUIREMENTS_CHECKLIST.md
- current codebase
- existing test files, especially:
  - agentcore/client_test.go
  - internal/transport/*_test.go
  - internal/registry/*_test.go if present

Generate or update unit tests only for implemented Phase 5 code.

Requirements to focus on:
- NATS-LIB-04
- NATS-LIB-16
- NATS-LIB-17
- NATS-LIB-23
- NATS-LIB-25

Add or extend tests for:
- agentcore/client.go / handler registration APIs
- internal/transport subscribe and callback binding code
- internal/registry subscription registry code
- minimal session reconnect-restore hook behavior if Phase 5 added one

Important existing test correction:
- Update TestPhase4RuntimeAndDeferredMethodsReturnExpectedErrors.
- RegisterConfigureHandler, RegisterActionHandler, RegisterResultHandler, and RegisterStatusHandler should no longer expect a non-nil “not implemented” error when called with valid input before Start(ctx).
- Phase 5 implements deferred handler registration, so valid pre-Start registration should succeed and store subscription intent.
- Preserve the rest of the Phase 4 test expectations unless behavior actually changed.

General test design rules:
- Test only implemented behavior.
- Do not start a real nats-server.
- Do not require Docker.
- Do not add integration tests.
- Do not test live reconnect against a real server.
- Use small test doubles where needed.
- Do not refactor production code only for tests unless a tiny behavior-neutral seam is absolutely required.
- Keep tests readable, deterministic, and focused.

Priority 1: public handler registration behavior

Cover:
- RegisterConfigureHandler
- RegisterActionHandler
- RegisterResultHandler
- RegisterStatusHandler

Include tests for:
- valid handler registration before Start(ctx) succeeds
- nil handler returns validation error
- invalid target returns validation error
- invalid action returns validation error for action handler
- registration after Close(ctx) follows current lifecycle behavior
- valid registration does not require an active NATS connection
- registration stores deferred subscription intent where observable through public behavior or package-local seams

Priority 2: subscription registry

Cover:
- adding configure/action/result/status intents
- duplicate behavior according to implementation policy
- ActivateAll / RestoreAll behavior with fake subscriber
- RestoreAll does not create duplicate active subscriptions
- DeactivateAll / CloseAll clears active subscription handles
- registry is safe for repeated deactivate/close calls

Priority 3: callback binding

Cover:
- valid configure payload decodes and invokes configure handler
- valid action payload decodes and invokes action handler
- valid result payload decodes and invokes result handler
- valid status payload decodes and invokes status handler
- malformed JSON does not call handler
- validation failure does not call handler
- user handler error is surfaced through existing error sink/logger behavior if implemented
- callback binding does not panic on nil or malformed message input where applicable

Priority 4: rpc_id preservation

Cover:
- result callback receives ResultEnvelope with rpc_id unchanged
- action/configure/status preserve rpc_id where the public model contains rpc_id
- configure receive path preserves uuid/config identity where applicable

Priority 5: subject usage

Cover:
- configure registration uses cmd.configure.<target>
- action registration uses cmd.action.<target>.<action>
- result registration uses result.<target>
- status registration uses status.<target>
- subject generation uses centralized helpers indirectly through observed subject values

Scope exclusions:
- Do not add real NATS integration tests
- Do not test live JetStream/KV behavior
- Do not test host/VyOS business logic
- Do not add examples or README changes
- Do not implement new production features beyond tiny fixes required for tests to compile/pass

Build / verification requirements:
- Run gofmt on all changed Go files
- Run go test ./...
- If go test ./... fails because of pre-existing unrelated issues, report them clearly
- Do not leave the repository in a non-compiling state

At the end, provide:
- files added
- files modified
- existing tests changed and why
- positive coverage summary
- negative coverage summary
- requirements covered:
  - NATS-LIB-04
  - NATS-LIB-16
  - NATS-LIB-17
  - NATS-LIB-23
  - NATS-LIB-25
- intentionally deferred gaps
