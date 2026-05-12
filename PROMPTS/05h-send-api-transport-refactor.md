Read the current PR/codebase carefully before making changes.

This is a Phase 5 design-correctness refactor prompt.

The public send/publish APIs were recently implemented:

- SubmitConfigure(...)
- SubmitAction(...)
- PublishResult(...)
- PublishStatus(...)

The behavior is directionally correct, but the implementation currently places too much transport responsibility directly inside agentcore/client.go.

agentcore/client.go currently performs lower-level publish work directly:
- validation
- JSON marshal
- NATS connection lookup
- nc.Publish(...)
- FlushWithContext(...)
- publish timeout handling
- publish metrics
- publish error construction

At the same time, internal/transport already contains publish/configure helpers, but those helpers currently import agentcore, which prevents agentcore from importing internal/transport without an import cycle.

The goal of this prompt is to fix the package design cleanly.

Do not change public API signatures.
Do not change public model structs or JSON tags.
Do not change public behavior.
Do not add workload/business logic.
Do not add observability redesign.
Do not add worker pools.
Do not add new sender/receiver integration tests in this prompt.
A separate prompt will add unit and integration tests after this design cleanup.

The desired architecture is:

    agentcore public facade
        -> internal/transport
        -> internal/session / nats.go

Where:
- agentcore owns public APIs and public model types.
- internal/transport owns low-level NATS publish mechanics.
- internal/session owns NATS connection lifecycle.
- internal/subjects owns subject generation.
- internal/contract can remain as-is unless a minimal cleanup is required.
- internal/runtimeerr owns internal typed errors.
- agentcore converts internal errors to public agentcore.Error at the facade boundary.

The important design rule:

    internal/transport must not import agentcore.

This is required so agentcore can import internal/transport without an import cycle.

---

## Primary objective

Refactor the current public send/publish implementation so that:

1. agentcore.Client public methods remain the facade:
   - SubmitConfigure(...)
   - SubmitAction(...)
   - PublishResult(...)
   - PublishStatus(...)

2. internal/transport owns the low-level publish mechanics:
   - NATS connection lookup
   - nc.Publish(...)
   - FlushWithContext(...)
   - publish timeout handling
   - publish metrics calls
   - internal publish error wrapping

3. agentcore no longer directly calls:
   - nc.Publish(...)
   - nc.FlushWithContext(...)

4. internal/transport no longer imports:
   - github.com/routerarchitects/nats-agent-core/agentcore

5. No import cycles exist.

6. Existing public behavior remains the same.

---

## Current issue to fix

The current implementation in agentcore/client.go includes a method like:

    func (c *Client) publishPayload(ctx context.Context, op, kind, subject string, payload []byte) error {
        nc, err := c.session.Connection()
        ...
        nc.Publish(subject, payload)
        ...
        nc.FlushWithContext(flushCtx)
        ...
    }

This should not live in the public facade.

Move this responsibility into internal/transport.

agentcore/client.go should become thinner:

- validate public input
- build/store public models
- build subject using internal/subjects
- JSON encode payload or call a local encode helper
- delegate actual publish/flush to internal/transport
- convert internal errors to public errors using toPublicError(...)

---

## Required package dependency direction

After the refactor, the dependency direction must be valid:

    agentcore
        imports internal/transport
        imports internal/subjects
        imports internal/session
        imports internal/kv
        imports internal/registry
        imports internal/runtimeerr

    internal/transport
        may import nats.go
        may import internal/runtimeerr
        may import time/context
        must not import agentcore

Do not make this dependency direction:

    internal/transport -> agentcore

That would recreate the import cycle.

Also avoid this unless you intentionally refactor all dependencies safely:

    agentcore -> internal/contract -> agentcore

If internal/contract still imports agentcore, do not import internal/contract from agentcore.

---

## Recommended transport design

Create or refactor internal/transport so it provides a low-level publisher independent of public facade types.

Suggested design:

    package transport

    type ConnectionProvider interface {
        Connection() (*nats.Conn, error)
    }

    type PublishMetrics interface {
        IncPublish(kind, subject, result string)
        ObservePublishLatency(kind, subject string, d time.Duration)
    }

    type Publisher struct {
        conn           ConnectionProvider
        publishTimeout func() time.Duration
        metrics        PublishMetrics
    }

    func NewPublisher(
        conn ConnectionProvider,
        publishTimeout func() time.Duration,
        metrics PublishMetrics,
    ) (*Publisher, error)

    func (p *Publisher) Publish(
        ctx context.Context,
        op string,
        kind string,
        subject string,
        payload []byte,
    ) error

This is only a suggested shape. Use the best idiomatic approach for the existing codebase, but keep the same boundaries.

Important:
- transport.Publisher should not know about ConfigureCommand, ActionCommand, ResultEnvelope, StatusEnvelope, or SubmissionAck.
- transport.Publisher should only publish an already-encoded payload to a subject.
- transport.Publisher should not construct public ACKs.
- transport.Publisher should not import agentcore.

---

## Error handling in internal/transport

internal/transport should return internal typed errors, preferably runtimeerr.Error.

Use existing runtimeerr codes where available.

Expected behavior:

- nil publisher dependencies -> validation error
- nil ctx -> validation error or use clear internal validation error
- canceled ctx -> clear error wrapping ctx.Err()
- missing subject -> validation error
- nil/empty payload -> validation error if appropriate
- disconnected connection -> propagate/return disconnected runtime error
- Publish failure -> publish_failed runtime error
- Flush failure -> publish_failed runtime error

Suggested internal op names:
- publish_payload
- submit_configure_publish_notification
- submit_action_publish
- publish_result
- publish_status

Returned errors should include:
- Op
- Subject
- Message
- Retryable
- Err where available

agentcore should convert runtimeerr.Error to agentcore.Error with existing toPublicError(...).

Do not return agentcore.Error from internal/transport.

---

## Metrics behavior

Keep existing metrics behavior but move it into internal/transport publish logic.

Current public metrics interface is in agentcore, but internal/transport must not import agentcore.

Use a structurally compatible local interface in internal/transport:

    type PublishMetrics interface {
        IncPublish(kind, subject, result string)
        ObservePublishLatency(kind, subject string, d time.Duration)
    }

agentcore.Metrics should satisfy this interface automatically because Go interfaces are structural.

Do not add new public metrics methods here.
Do not fix receive-side latency metric naming in this prompt.
That remains Phase 6 observability work.

---

## Context and timeout behavior

Keep current behavior:

- Public APIs validate ctx before calling transport.
- internal/transport should still be defensive if ctx is nil or canceled.
- Publish should call nc.Publish(...)
- Publish should call FlushWithContext(...)
- If ctx already has a deadline, use it.
- If ctx has no deadline, apply configured publish timeout.
- If configured publish timeout is zero or negative, use the caller context as-is.

Suggested helper inside internal/transport:

    func publishContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc)

Do not block forever when a publish timeout is configured.

---

## Client wiring

Add a private publisher field to Client if needed:

    publisher *transport.Publisher

In New(...), create the publisher after the session manager is created.

Example shape:

    publisher, err := transport.NewPublisher(
        runtime,
        func() time.Duration {
            return runtime.EffectiveConfig().Timeouts.PublishTimeout
        },
        options.metrics,
    )
    if err != nil {
        return nil, toPublicError(err)
    }

    client := &Client{
        ...
        publisher: publisher,
    }

This requires session.Manager to satisfy the transport connection provider through:

    Connection() (*nats.Conn, error)

It already has Connection(), so prefer using that.

Do not expose publisher publicly.

---

## Public API behavior to preserve

### SubmitConfigure(ctx, cmd)

Must continue to:

1. validate ctx
2. validate ConfigureCommand
3. require active runtime connection
4. store desired config in KV
5. build configure notification
6. publish notification to configured configure subject
7. return SubmissionAck with:
   - Accepted = true
   - RPCID
   - Target
   - Subject
   - AcceptedAt
   - KVBucket
   - KVKey
   - KVRevision

Keep store-then-notify behavior.

Do not publish configure command directly.
Do not wait for target result.
Do not add correlation store.

### SubmitAction(ctx, cmd)

Must continue to:

1. validate ctx
2. validate ActionCommand
3. publish action command to configured action subject
4. return SubmissionAck with:
   - Accepted = true
   - RPCID
   - Target
   - Subject
   - AcceptedAt

Do not wait for result.

### PublishResult(ctx, msg)

Must continue to:

1. validate ctx
2. validate ResultEnvelope
3. publish to configured result subject
4. preserve rpc_id exactly
5. return nil on successful publish/flush

### PublishStatus(ctx, msg)

Must continue to:

1. validate ctx
2. validate StatusEnvelope
3. publish to configured status subject
4. preserve optional rpc_id exactly
5. return nil on successful publish/flush

Status rpc_id should remain optional if current validation treats it as optional.

---

## Validation ownership

For this refactor, do not perform a large validation-package redesign unless required to break cycles.

It is acceptable for agentcore facade methods to keep public model validation for now, because those methods own the public model boundary.

However:
- do not duplicate new validation unnecessarily
- do not introduce behavior drift from existing validation
- do not make validation weaker
- do not make optional status rpc_id required

If a validator already exists in agentcore and is currently used by both send and receive paths, keep it stable.

If a validator currently exists only in internal/contract but cannot be imported without cycles, do not force agentcore to import internal/contract in this refactor.

The main goal here is transport dependency cleanup, not contract package redesign.

---

## What to do with current internal/transport high-level helpers

Inspect:

- internal/transport/publish.go
- internal/transport/configure.go
- internal/transport/*_test.go

If internal/transport currently contains high-level helpers that import agentcore, refactor them.

Preferred outcome:

- internal/transport should become a low-level transport package.
- Remove or rewrite high-level model-aware helpers from internal/transport if they require importing agentcore.
- Keep only helpers that do not import agentcore.

If existing internal/transport tests depend on old high-level helpers:
- update those tests minimally to test the new low-level publisher behavior
- keep meaningful coverage
- do not delete test coverage without replacement

Do not keep unused high-level helpers that import agentcore just to preserve old tests.

---

## Avoid these bad outcomes

Do not leave this import direction:

    internal/transport -> agentcore

Do not leave both of these as competing implementations:

    agentcore/client.go direct publish/flush
    internal/transport publish/flush

Do not solve the problem by copying more transport code into agentcore.

Do not introduce global state.

Do not introduce background goroutines.

Do not introduce worker pools.

Do not change public API signatures.

Do not change JSON contract fields.

Do not add durable JetStream consumers.

Do not add request/reply result waiting.

Do not implement Phase 6 observability changes.

---

## Expected final code shape

After refactor, agentcore/client.go should look conceptually like this:

    func (c *Client) SubmitAction(ctx context.Context, cmd ActionCommand) (*SubmissionAck, error) {
        const op = "submit_action"

        if err := validateOperationContext(op, ctx); err != nil {
            return nil, err
        }
        if err := validateActionCommand(op, cmd); err != nil {
            return nil, err
        }

        subject, err := c.subjects.ActionSubject(cmd.Target, cmd.Action)
        if err != nil {
            return nil, toPublicError(err)
        }

        payload, err := json.Marshal(cmd)
        if err != nil {
            return nil, public encode error
        }

        if err := c.publisher.Publish(ctx, "submit_action_publish", "action", subject, payload); err != nil {
            return nil, toPublicError(err)
        }

        return &SubmissionAck{...}, nil
    }

The exact implementation may differ, but the key is:

    c.publisher.Publish(...)

not:

    nc.Publish(...)
    nc.FlushWithContext(...)

inside agentcore.

---

## Existing tests

Existing tests must still pass.

You may update existing internal/transport tests if they target old high-level transport helpers.

You may update existing client tests minimally if required by refactor.

Do not add the full new public sender unit test suite here.
Do not add new sender-to-receiver integration tests here.
Those will be handled in the next prompt.

---

## Verification commands

Run:

    gofmt on all changed Go files
    go test ./...

If feasible, also run:

    go test -race ./...

Do not claim a command passed unless it was actually run.

If a command fails, fix the code or clearly report the failure.

---

## Final response required from Codex

After coding, summarize:

1. Files changed
2. How the import cycle risk was removed
3. Whether internal/transport still imports agentcore
4. What internal/transport now owns
5. What agentcore.Client now owns
6. How SubmitConfigure delegates publish mechanics
7. How SubmitAction delegates publish mechanics
8. How PublishResult delegates publish mechanics
9. How PublishStatus delegates publish mechanics
10. Whether public API signatures changed
11. Whether public behavior changed
12. Existing tests updated, if any
13. Commands run and results
14. Confirmation that no workload/business logic was added
15. Confirmation that Phase 6 observability redesign was not added
