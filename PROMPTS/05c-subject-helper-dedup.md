Read the current codebase carefully.

Focus only on cleaning up the Phase 5 subject helper duplication.

Do not add new feature scope.
Do not change public API signatures.
Do not add integration tests.
Do not add workload/business logic.
Do not redesign handler registration, subscription registry, reconnect restore, or callback binding behavior.

Context:
Phase 5 added receive-side handler registration and subscription support in:

- agentcore/handlers_runtime.go
- internal/registry/*
- internal/session/*

During review, one remaining cleanup was identified:

Subject helper duplication remains.

agentcore/handlers_runtime.go currently defines local subject defaults, subject pattern resolution, subject pattern validation, and subject token validation logic, even though the repo already has centralized subject helpers under internal/subjects.

The goal of this corrective patch is to make Phase 5 handler registration use the centralized internal/subjects helpers instead of duplicating subject-building logic in agentcore.

Requirements:
- Keep existing Phase 5 behavior unchanged.
- Keep all existing public handler registration APIs unchanged:
  - RegisterConfigureHandler(...)
  - RegisterActionHandler(...)
  - RegisterResultHandler(...)
  - RegisterStatusHandler(...)
- Keep target-first routing unchanged:
  - cmd.configure.<target>
  - cmd.action.<target>.<action>
  - result.<target>
  - status.<target>
- Keep configured subject pattern behavior unchanged.
- Keep validation behavior equivalent or stricter only where already intended by the centralized subjects package.
- Keep queue-group behavior unchanged.
- Keep registry behavior unchanged.
- Keep callback binding behavior unchanged.
- Keep reconnect restore behavior unchanged.
- Keep tests passing.

Important dependency issue:
The current internal/subjects package may import agentcore.SubjectConfig.
If that prevents agentcore from importing internal/subjects due to an import cycle, fix the dependency direction cleanly.

Preferred approach:
1. Move subject config conversion out of internal/subjects if needed.
2. Make internal/subjects independent from agentcore.
3. Define a package-local config type in internal/subjects if needed, for example:
   - subjects.Config
   - subjects.Patterns
4. In agentcore, convert public SubjectConfig into internal/subjects.Patterns or internal/subjects.Config.
5. Use subjects.NewBuilder(...) or equivalent centralized builder from agentcore handler registration code.

Do not solve the import cycle by keeping duplicated logic in agentcore.

Expected production changes:
- Remove duplicated subject constants from agentcore/handlers_runtime.go if they are already available in internal/subjects.
- Remove duplicated subject pattern validation from agentcore/handlers_runtime.go.
- Remove duplicated target/action token validation from agentcore/handlers_runtime.go.
- Replace subscriptionSubjectPatterns helper logic with use of internal/subjects.Builder or equivalent.
- Update Client to hold a subject builder or resolved subject helper instead of duplicated local patterns.
- Ensure New(cfg, ...) still validates configured subject patterns early.
- Ensure Register*Handler still returns typed public errors through existing conversion helpers.
- Keep error codes compatible with existing tests.

Potential files to update:
- internal/subjects/subjects.go
- internal/subjects/validation.go if present
- agentcore/client.go
- agentcore/handlers_runtime.go
- agentcore/handlers_runtime_test.go
- internal/subjects/*_test.go if existing tests need small updates

Implementation details:
- internal/subjects must not import agentcore after this cleanup.
- agentcore may import internal/subjects.
- If internal/subjects.PatternsFromConfig currently accepts agentcore.SubjectConfig, replace it with a dependency-free internal type or helper.
- Add an adapter in agentcore if needed:
  - toSubjectPatterns(cfg SubjectConfig) subjects.Patterns
- Keep centralized subject functions responsible for:
  - default patterns
  - pattern validation
  - target validation
  - action validation
  - configure/action/result/status subject construction

Testing requirements:
- Unit tests only.
- Do not start nats-server.
- Do not require Docker.
- Do not add integration tests.

Update or add tests to verify:
1. Handler registration still stores expected subjects:
   - cmd.configure.vyos
   - cmd.action.vyos.trace
   - result.vyos
   - status.vyos
2. Custom subject patterns from Config.Subjects still work.
3. Invalid target/action values still fail.
4. Invalid configured subject patterns still fail during New(...).
5. No import cycle exists.
6. Existing internal/subjects tests still pass.
7. Existing Phase 5 handler registration tests still pass.

Do not over-refactor tests.
Prefer adjusting existing tests only where behavior or dependency direction changed.

Build / verification:
- Run gofmt on all changed Go files.
- Run go test ./...
- Run go test -race ./...
- If go test ./... fails because of unrelated pre-existing issues, report that clearly.
- Do not leave the repository in a non-compiling state.

After coding, summarize:
1. files changed
2. how subject helper duplication was removed
3. how import-cycle risk was resolved
4. how handler registration now uses centralized subject helpers
5. tests added or updated
6. confirmation that public APIs did not change
7. confirmation that handler registration, registry, reconnect restore, and callback binding behavior were not redesigned
