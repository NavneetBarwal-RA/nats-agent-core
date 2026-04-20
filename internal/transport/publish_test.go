package transport

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/nats-agent-core/internal/contract"
	"github.com/routerarchitects/nats-agent-core/internal/subjects"
)

type publishCall struct {
	subject string
	payload []byte
}

type stubPublisher struct {
	calls     []publishCall
	err       error
	onPublish func(subject string, payload []byte)
}

func (p *stubPublisher) Publish(_ context.Context, subject string, payload []byte) error {
	copied := append([]byte(nil), payload...)
	p.calls = append(p.calls, publishCall{subject: subject, payload: copied})
	if p.onPublish != nil {
		p.onPublish(subject, copied)
	}
	return p.err
}

func transportTestTime() time.Time {
	return time.Unix(1700000000, 0).UTC()
}

func transportRawJSON(s string) json.RawMessage {
	return json.RawMessage(s)
}

func validTransportConfigureNotification() agentcore.ConfigureNotification {
	return agentcore.ConfigureNotification{
		Version:     "1.0",
		RPCID:       "rpc-notify-1",
		Target:      "vyos",
		CommandType: "configure",
		UUID:        "cfg-001",
		KVBucket:    "cfg_desired",
		KVKey:       "desired.vyos",
		Timestamp:   transportTestTime(),
	}
}

func validTransportActionCommand() agentcore.ActionCommand {
	return agentcore.ActionCommand{
		Version:     "1.0",
		RPCID:       "rpc-action-1",
		Target:      "vyos",
		CommandType: "action",
		Action:      "trace",
		Payload:     transportRawJSON(`{"destination":"8.8.8.8"}`),
		Timestamp:   transportTestTime(),
	}
}

func validTransportResultEnvelope() agentcore.ResultEnvelope {
	return agentcore.ResultEnvelope{
		Version:   "1.0",
		RPCID:     "rpc-result-1",
		Target:    "vyos",
		Result:    "success",
		Message:   "ok",
		Payload:   transportRawJSON(`{"applied":true}`),
		Timestamp: transportTestTime(),
	}
}

func validTransportStatusEnvelope() agentcore.StatusEnvelope {
	return agentcore.StatusEnvelope{
		Version:   "1.0",
		Target:    "vyos",
		Status:    "running",
		Message:   "ready",
		Payload:   transportRawJSON(`{"ready":true}`),
		Timestamp: transportTestTime(),
	}
}

func requireTransportAgentcoreError(t *testing.T, err error, wantCode agentcore.Code, wantOp string, wantMsgPart string) *agentcore.Error {
	t.Helper()

	if err == nil {
		t.Fatal("expected non-nil error")
	}

	var got *agentcore.Error
	if !errors.As(err, &got) {
		t.Fatalf("expected *agentcore.Error, got %T", err)
	}
	if got.Code != wantCode {
		t.Fatalf("expected error code %q, got %q", wantCode, got.Code)
	}
	if got.Op != wantOp {
		t.Fatalf("expected error op %q, got %q", wantOp, got.Op)
	}
	if wantMsgPart != "" && !strings.Contains(got.Message, wantMsgPart) {
		t.Fatalf("expected error message to contain %q, got %q", wantMsgPart, got.Message)
	}
	return got
}

/*
TC-TRANSPORT-PUBLISH-001
Type: Positive
Title: NewPublishPaths accepts a valid subject builder
Summary:
Verifies that publish path construction succeeds with a valid builder and
initializes internal dependencies for publish helpers.

Validates:
  - non-nil publish paths are returned
  - internal clock source is initialized
*/
func TestNewPublishPathsAcceptsValidBuilder(t *testing.T) {
	builder := subjects.NewDefaultBuilder()

	paths, err := NewPublishPaths(builder)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if paths == nil {
		t.Fatal("expected non-nil publish paths")
	}
	if paths.now == nil {
		t.Fatal("expected publish paths clock to be initialized")
	}
}

/*
TC-TRANSPORT-PUBLISH-002
Type: Negative
Title: NewPublishPaths rejects nil builder
Summary:
Verifies that publish path construction fails fast when subject builder
dependency is missing.

Validates:
  - nil builder returns CodeValidation
  - error op is new_publish_paths
*/
func TestNewPublishPathsRejectsNilBuilder(t *testing.T) {
	_, err := NewPublishPaths(nil)

	requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "new_publish_paths", "subject builder is required")
}

/*
TC-TRANSPORT-PUBLISH-003
Type: Positive
Title: PublishConfigureNotification publishes encoded notification on configure subject
Summary:
Verifies that configure notification publish uses the configured subject helper
and sends an encoded notification payload.

Validates:
  - publish uses cmd.configure.<target> subject
  - payload decodes as ConfigureNotification with preserved fields
*/
func TestPublishConfigureNotificationPublishesEncodedNotification(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	msg := validTransportConfigureNotification()
	pub := &stubPublisher{}

	err = paths.PublishConfigureNotification(context.Background(), pub, msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].subject != "cmd.configure.vyos" {
		t.Fatalf("expected subject %q, got %q", "cmd.configure.vyos", pub.calls[0].subject)
	}

	decoded, err := contract.DecodeConfigureNotification(pub.calls[0].payload)
	if err != nil {
		t.Fatalf("expected decodable configure notification payload, got %v", err)
	}
	if decoded.RPCID != msg.RPCID {
		t.Fatalf("expected RPCID %q, got %q", msg.RPCID, decoded.RPCID)
	}
	if decoded.KVBucket != msg.KVBucket {
		t.Fatalf("expected KVBucket %q, got %q", msg.KVBucket, decoded.KVBucket)
	}
	if decoded.KVKey != msg.KVKey {
		t.Fatalf("expected KVKey %q, got %q", msg.KVKey, decoded.KVKey)
	}
}

/*
TC-TRANSPORT-PUBLISH-004
Type: Negative
Title: PublishConfigureNotification rejects nil publisher
Summary:
Verifies that configure notification publish returns a validation error when
publisher dependency is missing.

Validates:
  - nil publisher returns CodeValidation
  - error op is publish_configure_notification
*/
func TestPublishConfigureNotificationRejectsNilPublisher(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	err = paths.PublishConfigureNotification(context.Background(), nil, validTransportConfigureNotification())
	requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "publish_configure_notification", "publisher is required")
}

/*
TC-TRANSPORT-PUBLISH-005
Type: Negative
Title: PublishConfigureNotification wraps publisher failure
Summary:
Verifies that configure notification publish wraps low-level publisher failures
as typed publish errors.

Validates:
  - publisher failure returns CodePublishFailed
  - wrapped cause is preserved for error inspection
*/
func TestPublishConfigureNotificationWrapsPublisherFailure(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	cause := errors.New("write timeout")
	pub := &stubPublisher{err: cause}

	err = paths.PublishConfigureNotification(context.Background(), pub, validTransportConfigureNotification())
	got := requireTransportAgentcoreError(t, err, agentcore.CodePublishFailed, "publish_configure_notification", "publish failed")
	if !errors.Is(got, cause) {
		t.Fatal("expected wrapped publish cause to be reachable via errors.Is")
	}
}

/*
TC-TRANSPORT-PUBLISH-006
Type: Positive
Title: SubmitAction publishes command and returns submission ack
Summary:
Verifies that action submit publishes the encoded action command and returns a
successful submission ack with shared correlation fields.

Validates:
  - action publish uses cmd.action.<target>.<action> subject
  - submission ack contains accepted fields and internal AcceptedAt time
  - published payload decodes as ActionCommand
*/
func TestSubmitActionPublishesCommandAndReturnsSubmissionAck(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	ackTime := transportTestTime().Add(5 * time.Minute)
	paths.now = func() time.Time { return ackTime }

	cmd := validTransportActionCommand()
	cmd.Timestamp = transportTestTime().Add(-1 * time.Hour)

	pub := &stubPublisher{}

	ack, err := paths.SubmitAction(context.Background(), pub, cmd)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ack == nil {
		t.Fatal("expected non-nil submission ack")
	}
	if !ack.Accepted {
		t.Fatal("expected ack Accepted=true")
	}
	if ack.RPCID != cmd.RPCID {
		t.Fatalf("expected ack RPCID %q, got %q", cmd.RPCID, ack.RPCID)
	}
	if ack.Target != cmd.Target {
		t.Fatalf("expected ack Target %q, got %q", cmd.Target, ack.Target)
	}
	if ack.Subject != "cmd.action.vyos.trace" {
		t.Fatalf("expected ack Subject %q, got %q", "cmd.action.vyos.trace", ack.Subject)
	}
	if !ack.AcceptedAt.Equal(ackTime) {
		t.Fatalf("expected ack AcceptedAt %v, got %v", ackTime, ack.AcceptedAt)
	}
	if ack.AcceptedAt.Equal(cmd.Timestamp) {
		t.Fatal("expected AcceptedAt to use internal clock, not command timestamp")
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].subject != "cmd.action.vyos.trace" {
		t.Fatalf("expected subject %q, got %q", "cmd.action.vyos.trace", pub.calls[0].subject)
	}

	decoded, err := contract.DecodeActionCommand(pub.calls[0].payload)
	if err != nil {
		t.Fatalf("expected decodable action payload, got %v", err)
	}
	if decoded.Action != cmd.Action {
		t.Fatalf("expected Action %q, got %q", cmd.Action, decoded.Action)
	}
}

/*
TC-TRANSPORT-PUBLISH-007
Type: Negative
Title: SubmitAction wraps publish failure
Summary:
Verifies that action submit returns a typed publish failure and no successful
ack when the publisher returns an error.

Validates:
  - publish error returns CodePublishFailed
  - submit_action_publish op is preserved
*/
func TestSubmitActionWrapsPublishFailure(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	pub := &stubPublisher{err: errors.New("nats unavailable")}

	ack, err := paths.SubmitAction(context.Background(), pub, validTransportActionCommand())
	if ack != nil {
		t.Fatalf("expected nil ack, got %#v", ack)
	}
	requireTransportAgentcoreError(t, err, agentcore.CodePublishFailed, "submit_action_publish", "publish failed")
}

/*
TC-TRANSPORT-PUBLISH-008
Type: Positive
Title: PublishResult publishes encoded result envelope
Summary:
Verifies that result publish uses the result subject helper and sends an
encoded result envelope payload.

Validates:
  - publish uses result.<target> subject
  - payload decodes as ResultEnvelope
*/
func TestPublishResultPublishesEncodedResultEnvelope(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	msg := validTransportResultEnvelope()
	pub := &stubPublisher{}

	err = paths.PublishResult(context.Background(), pub, msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].subject != "result.vyos" {
		t.Fatalf("expected subject %q, got %q", "result.vyos", pub.calls[0].subject)
	}

	decoded, err := contract.DecodeResultEnvelope(pub.calls[0].payload)
	if err != nil {
		t.Fatalf("expected decodable result payload, got %v", err)
	}
	if decoded.Result != msg.Result {
		t.Fatalf("expected Result %q, got %q", msg.Result, decoded.Result)
	}
}

/*
TC-TRANSPORT-PUBLISH-009
Type: Positive
Title: PublishStatus publishes encoded status envelope
Summary:
Verifies that status publish uses the status subject helper and sends an
encoded status envelope payload.

Validates:
  - publish uses status.<target> subject
  - payload decodes as StatusEnvelope
*/
func TestPublishStatusPublishesEncodedStatusEnvelope(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	msg := validTransportStatusEnvelope()
	pub := &stubPublisher{}

	err = paths.PublishStatus(context.Background(), pub, msg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].subject != "status.vyos" {
		t.Fatalf("expected subject %q, got %q", "status.vyos", pub.calls[0].subject)
	}

	decoded, err := contract.DecodeStatusEnvelope(pub.calls[0].payload)
	if err != nil {
		t.Fatalf("expected decodable status payload, got %v", err)
	}
	if decoded.Status != msg.Status {
		t.Fatalf("expected Status %q, got %q", msg.Status, decoded.Status)
	}
}

/*
TC-TRANSPORT-PUBLISH-010
Type: Negative
Title: publishEncoded rejects nil publisher
Summary:
Verifies that the shared publish helper rejects nil publisher dependency before
attempting to publish.

Validates:
  - nil publisher returns CodeValidation
  - helper op and subject are preserved in the error
*/
func TestPublishEncodedRejectsNilPublisher(t *testing.T) {
	err := publishEncoded(context.Background(), nil, "publish_result", "result.vyos", []byte(`{}`))
	got := requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "publish_result", "publisher is required")
	if got.Subject != "result.vyos" {
		t.Fatalf("expected subject %q, got %q", "result.vyos", got.Subject)
	}
}

/*
TC-TRANSPORT-PUBLISH-011
Type: Negative
Title: publishEncoded wraps publisher failure
Summary:
Verifies that the shared publish helper wraps publisher failures with publish
error semantics used by higher-level wrappers.

Validates:
  - publisher failure returns CodePublishFailed
  - wrapped cause remains accessible
*/
func TestPublishEncodedWrapsPublisherFailure(t *testing.T) {
	cause := errors.New("publish failed")
	pub := &stubPublisher{err: cause}

	err := publishEncoded(context.Background(), pub, "publish_result", "result.vyos", []byte(`{}`))
	got := requireTransportAgentcoreError(t, err, agentcore.CodePublishFailed, "publish_result", "publish failed")
	if !errors.Is(got, cause) {
		t.Fatal("expected wrapped publish cause to be reachable via errors.Is")
	}
}

/*
TC-TRANSPORT-PUBLISH-012
Type: Negative
Title: SubmitAction rejects invalid target and action before publish
Summary:
Verifies that action submit validates subject inputs before attempting publish
and returns validation failures for malformed target or action tokens.

Validates:
  - invalid target returns validate_target error
  - invalid action returns validate_action error
  - publish side effect is skipped on validation failures
*/
func TestSubmitActionRejectsInvalidTargetAndActionBeforePublish(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*agentcore.ActionCommand)
		wantOp  string
		msgPart string
	}{
		{
			name: "invalid target",
			mutate: func(cmd *agentcore.ActionCommand) {
				cmd.Target = "vyos core"
			},
			wantOp:  "validate_target",
			msgPart: "target cannot contain whitespace",
		},
		{
			name: "invalid action",
			mutate: func(cmd *agentcore.ActionCommand) {
				cmd.Action = "trace now"
			},
			wantOp:  "validate_action",
			msgPart: "action cannot contain whitespace",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := validTransportActionCommand()
			tc.mutate(&cmd)

			pub := &stubPublisher{}
			ack, err := paths.SubmitAction(context.Background(), pub, cmd)
			if ack != nil {
				t.Fatalf("expected nil ack, got %#v", ack)
			}
			requireTransportAgentcoreError(t, err, agentcore.CodeValidation, tc.wantOp, tc.msgPart)
			if len(pub.calls) != 0 {
				t.Fatalf("expected publish not to be called, got %d calls", len(pub.calls))
			}
		})
	}
}

/*
TC-TRANSPORT-PUBLISH-013
Type: Negative
Title: PublishResult rejects invalid target before publish
Summary:
Verifies that result publish validates target before encoding and publishing.

Validates:
  - invalid target returns validate_target error
  - publish side effect is skipped
*/
func TestPublishResultRejectsInvalidTargetBeforePublish(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	msg := validTransportResultEnvelope()
	msg.Target = "vyos core"
	pub := &stubPublisher{}

	err = paths.PublishResult(context.Background(), pub, msg)
	requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "validate_target", "target cannot contain whitespace")
	if len(pub.calls) != 0 {
		t.Fatalf("expected publish not to be called, got %d calls", len(pub.calls))
	}
}

/*
TC-TRANSPORT-PUBLISH-014
Type: Negative
Title: PublishStatus rejects invalid target before publish
Summary:
Verifies that status publish validates target before encoding and publishing.

Validates:
  - invalid target returns validate_target error
  - publish side effect is skipped
*/
func TestPublishStatusRejectsInvalidTargetBeforePublish(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	msg := validTransportStatusEnvelope()
	msg.Target = "vyos core"
	pub := &stubPublisher{}

	err = paths.PublishStatus(context.Background(), pub, msg)
	requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "validate_target", "target cannot contain whitespace")
	if len(pub.calls) != 0 {
		t.Fatalf("expected publish not to be called, got %d calls", len(pub.calls))
	}
}

/*
TC-TRANSPORT-PUBLISH-015
Type: Negative
Title: PublishResult wraps publisher failure
Summary:
Verifies that result publish wraps low-level publisher failures using the
publish_result operation context.

Validates:
  - publish failure returns CodePublishFailed
  - wrapped cause is preserved
*/
func TestPublishResultWrapsPublisherFailure(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	cause := errors.New("publish result failed")
	pub := &stubPublisher{err: cause}

	err = paths.PublishResult(context.Background(), pub, validTransportResultEnvelope())
	got := requireTransportAgentcoreError(t, err, agentcore.CodePublishFailed, "publish_result", "publish failed")
	if !errors.Is(got, cause) {
		t.Fatal("expected wrapped publish cause to be reachable via errors.Is")
	}
}

/*
TC-TRANSPORT-PUBLISH-016
Type: Negative
Title: PublishStatus wraps publisher failure
Summary:
Verifies that status publish wraps low-level publisher failures using the
publish_status operation context.

Validates:
  - publish failure returns CodePublishFailed
  - wrapped cause is preserved
*/
func TestPublishStatusWrapsPublisherFailure(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	cause := errors.New("publish status failed")
	pub := &stubPublisher{err: cause}

	err = paths.PublishStatus(context.Background(), pub, validTransportStatusEnvelope())
	got := requireTransportAgentcoreError(t, err, agentcore.CodePublishFailed, "publish_status", "publish failed")
	if !errors.Is(got, cause) {
		t.Fatal("expected wrapped publish cause to be reachable via errors.Is")
	}
}

/*
TC-TRANSPORT-PUBLISH-017
Type: Negative
Title: PublishConfigureNotification rejects invalid target before publish
Summary:
Verifies that configure notification publish validates target input before
encoding and publishing.

Validates:
  - invalid target returns validate_target error
  - publish side effect is skipped
*/
func TestPublishConfigureNotificationRejectsInvalidTargetBeforePublish(t *testing.T) {
	paths, err := NewPublishPaths(subjects.NewDefaultBuilder())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	msg := validTransportConfigureNotification()
	msg.Target = "vyos core"
	pub := &stubPublisher{}

	err = paths.PublishConfigureNotification(context.Background(), pub, msg)
	requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "validate_target", "target cannot contain whitespace")
	if len(pub.calls) != 0 {
		t.Fatalf("expected publish not to be called, got %d calls", len(pub.calls))
	}
}

/*
TC-TRANSPORT-PUBLISH-018
Type: Positive
Title: publishEncoded publishes successfully with valid dependencies
Summary:
Verifies that the shared publish helper succeeds and forwards subject and payload
to the publisher when all dependencies are valid.

Validates:
  - helper returns nil for a successful publish
  - publisher receives one call with expected subject and payload
*/
func TestPublishEncodedPublishesSuccessfullyWithValidDependencies(t *testing.T) {
	pub := &stubPublisher{}
	payload := []byte(`{"ok":true}`)

	err := publishEncoded(context.Background(), pub, "publish_result", "result.vyos", payload)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].subject != "result.vyos" {
		t.Fatalf("expected subject %q, got %q", "result.vyos", pub.calls[0].subject)
	}
	if string(pub.calls[0].payload) != string(payload) {
		t.Fatalf("expected payload %s, got %s", string(payload), string(pub.calls[0].payload))
	}
}
