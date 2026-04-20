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

Generate unit tests only for the implemented Phase 3 messaging code in:

- internal/subjects/subjects.go
- internal/subjects/validate.go
- internal/transport/publish.go
- internal/transport/configure.go

Requirements to focus on:
- NATS-LIB-12
- NATS-LIB-15
- NATS-LIB-20
- NATS-LIB-21
- NATS-LIB-22

Add tests in these files:
- internal/subjects/validate_test.go
- internal/subjects/subjects_test.go
- internal/transport/publish_test.go
- internal/transport/configure_test.go

Test only implemented behavior.

General test design rules:
- For each function under test, cover:
  1. valid success path
  2. direct validation failure
  3. dependency failure
  4. multi-step boundary failure if the function performs more than one operation
  5. side-effect assertions such as call count, call order, and publish/store not called when they should not be
- For every multi-step flow, add one test for each failure boundary, not only the first failure path
- If a function does validate -> store -> publish, test:
  - validation fails before store
  - store fails before publish
  - publish fails after successful store
  - success path
- On failure, assert whether returned ack/result should be nil where applicable
- Always assert whether downstream side effects were skipped or already performed
- Prefer wrapper-level tests, not only shared helper tests, for invalid input and publish failure paths
- Keep tests readable, deterministic, and focused
- Use small test doubles and explicit assertions rather than over-abstracting test helpers

Priority order:

1. Subject validation
Cover:
- ValidateTarget
- ValidateAction
- validatePattern

Include positive and negative tests for:
- valid target/action
- empty or whitespace-only values
- embedded whitespace
- '.'
- wildcard tokens '*' and '>'
- unsupported characters
- valid placeholder count
- invalid placeholder count
- unsupported format directives
- pattern whitespace
- wildcard in pattern

2. Subject builders
Cover:
- DefaultPatterns
- PatternsFromConfig
- NewBuilder
- NewDefaultBuilder
- ConfigureSubject
- ActionSubject
- ResultSubject
- StatusSubject
- HealthSubject

Include tests for:
- defaults are returned correctly
- empty config preserves defaults
- partial override preserves defaults for unspecified patterns
- valid overrides are accepted
- invalid patterns are rejected
- valid subjects are built correctly
- invalid target/action is rejected

3. Publish paths
Cover:
- NewPublishPaths
- PublishConfigureNotification
- SubmitAction
- PublishResult
- PublishStatus
- publishEncoded

Include tests for:
- nil builder rejected
- nil publisher rejected
- publish failure wrapped as CodePublishFailed
- success path publishes expected subject
- success path publishes encoded payload
- SubmitAction returns SubmissionAck with correct fields
- custom/internal clock is used for AcceptedAt if implementation supports it
- SubmitAction rejects invalid target before publish
- SubmitAction rejects invalid action before publish
- PublishResult rejects invalid target before publish
- PublishStatus rejects invalid target before publish
- PublishResult wraps publisher failure
- PublishStatus wraps publisher failure

4. Configure paths
Cover:
- NewConfigurePaths
- SubmitConfigure
- buildKVKey

Include tests for:
- nil store rejected
- nil publisher rejected
- default bucket/key pattern used when config is empty
- custom now function stored and used
- invalid configure command rejected before store
- store failure wrapped as CodeKVStoreFailed
- nil stored result rejected
- stored bucket/key used when present
- fallback bucket/key generation used when missing
- buildKVKey success path
- buildKVKey invalid pattern failures
- configure flow is store first, then notify
- notify publish does not happen when store fails
- store succeeds but notify publish fails
- publish failure after successful store returns CodePublishFailed
- ack is nil when notify publish fails
- store is still called exactly once before publish failure is returned
- configure publish failure does not appear as store failure
- returned SubmissionAck contains expected fields

Use small test doubles for:
- Publisher
- DesiredConfigStore

The test doubles should let you assert:
- call count
- call order
- published subject
- published payload
- returned errors
- stored result values

Do not add tests for:
- subscribe wrappers
- handler registration
- reconnect restore
- public agentcore.Client wiring
- receive-side result correlation
- NATS/JetStream integration
- future phases

At the end, provide:
- files added
- files modified
- positive coverage summary
- negative coverage summary
- any existing tests changed and why
- any intentionally deferred gaps
