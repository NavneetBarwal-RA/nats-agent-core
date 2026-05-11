package agentcore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/routerarchitects/nats-agent-core/internal/runtimeerr"
)

type sendPublishCall struct {
	ctx     context.Context
	op      string
	kind    string
	subject string
	payload []byte
}

type fakeSendPublisher struct {
	calls []sendPublishCall
	err   error
}

func (p *fakeSendPublisher) Publish(ctx context.Context, op, kind, subject string, payload []byte) error {
	p.calls = append(p.calls, sendPublishCall{
		ctx:     ctx,
		op:      op,
		kind:    kind,
		subject: subject,
		payload: append([]byte(nil), payload...),
	})
	return p.err
}

type fakeStoreDesiredConfig struct {
	calls  []DesiredConfigRecord
	stored *StoredDesiredConfig
	err    error
}

func (s *fakeStoreDesiredConfig) install(client *Client) {
	client.storeDesiredConfigFn = func(_ context.Context, rec DesiredConfigRecord) (*StoredDesiredConfig, error) {
		s.calls = append(s.calls, rec)
		if s.err != nil {
			return nil, s.err
		}
		return s.stored, nil
	}
}

func sendFixedNow() time.Time {
	return time.Unix(1700000000, 0).UTC()
}

func validSendConfigureCommand() ConfigureCommand {
	return ConfigureCommand{
		Version:   "1.0",
		RPCID:     "rpc-config-1",
		Target:    "vyos",
		UUID:      "cfg-1",
		Payload:   json.RawMessage(`{"hostname":"router-1"}`),
		Timestamp: sendFixedNow().Add(-5 * time.Minute),
	}
}

func validSendActionCommand() ActionCommand {
	return ActionCommand{
		Version:     "1.0",
		RPCID:       "rpc-action-1",
		Target:      "vyos",
		CommandType: "action",
		Action:      "trace",
		Payload:     json.RawMessage(`{"destination":"8.8.8.8"}`),
		Timestamp:   sendFixedNow(),
	}
}

func validSendResultEnvelope() ResultEnvelope {
	return ResultEnvelope{
		Version:     "1.0",
		RPCID:       "rpc-result-1",
		Target:      "vyos",
		CommandType: "configure",
		UUID:        "cfg-1",
		Action:      "trace",
		Result:      "success",
		Payload:     json.RawMessage(`{"applied":true}`),
		Timestamp:   sendFixedNow(),
	}
}

func validSendStatusEnvelope() StatusEnvelope {
	return StatusEnvelope{
		Version:   "1.0",
		RPCID:     "rpc-status-1",
		Target:    "vyos",
		Status:    "running",
		Stage:     "startup",
		Payload:   json.RawMessage(`{"ready":true}`),
		Timestamp: sendFixedNow(),
	}
}

func newSendTestClient(t *testing.T) (*Client, *fakeSendPublisher) {
	t.Helper()

	now := sendFixedNow()
	client, err := New(testConfig(), WithClock(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("New returned unexpected error: %v", err)
	}

	pub := &fakeSendPublisher{}
	client.publisher = pub
	return client, pub
}

/*
TC-CLIENT-SEND-001
Type: Positive
Title: SubmitAction publishes encoded action command and returns ack
Summary:
Verifies that SubmitAction validates a command, publishes it on the configured
action subject, and returns a successful submission ack with correlation fields.

Validates:
  - ack is accepted and preserves rpc_id target and subject
  - publish uses submit_action_publish operation and action kind
  - encoded published payload preserves action command fields
*/
func TestSubmitActionPublishesCommandAndReturnsAck(t *testing.T) {
	client, pub := newSendTestClient(t)
	cmd := validSendActionCommand()

	ack, err := client.SubmitAction(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ack == nil {
		t.Fatal("expected non-nil ack")
	}
	if !ack.Accepted {
		t.Fatal("expected ack.Accepted=true")
	}
	if ack.RPCID != cmd.RPCID {
		t.Fatalf("expected ack RPCID %q, got %q", cmd.RPCID, ack.RPCID)
	}
	if ack.Target != cmd.Target {
		t.Fatalf("expected ack Target %q, got %q", cmd.Target, ack.Target)
	}
	if ack.Subject != "cmd.action.vyos.trace" {
		t.Fatalf("expected ack subject %q, got %q", "cmd.action.vyos.trace", ack.Subject)
	}

	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}
	call := pub.calls[0]
	if call.op != "submit_action_publish" {
		t.Fatalf("expected publish op %q, got %q", "submit_action_publish", call.op)
	}
	if call.kind != "action" {
		t.Fatalf("expected publish kind %q, got %q", "action", call.kind)
	}
	if call.subject != "cmd.action.vyos.trace" {
		t.Fatalf("expected publish subject %q, got %q", "cmd.action.vyos.trace", call.subject)
	}

	var decoded ActionCommand
	if err := json.Unmarshal(call.payload, &decoded); err != nil {
		t.Fatalf("failed to decode published action payload: %v", err)
	}
	if decoded.RPCID != cmd.RPCID || decoded.Target != cmd.Target {
		t.Fatalf("unexpected action identity: %+v", decoded)
	}
	if decoded.Action != cmd.Action || decoded.CommandType != cmd.CommandType {
		t.Fatalf("unexpected action command fields: %+v", decoded)
	}
	if string(decoded.Payload) != string(cmd.Payload) {
		t.Fatalf("expected payload %s, got %s", string(cmd.Payload), string(decoded.Payload))
	}
	if !decoded.Timestamp.Equal(cmd.Timestamp) {
		t.Fatalf("expected timestamp %v, got %v", cmd.Timestamp, decoded.Timestamp)
	}
}

/*
TC-CLIENT-SEND-002
Type: Negative
Title: SubmitAction rejects invalid command before publishing
Summary:
Verifies that SubmitAction fails validation for invalid input and does not
invoke the transport publisher on validation errors.

Validates:
  - invalid command returns CodeValidation
  - ack is nil on validation failure
  - publish is not called
*/
func TestSubmitActionRejectsInvalidInputBeforePublish(t *testing.T) {
	client, pub := newSendTestClient(t)
	cmd := validSendActionCommand()
	cmd.Action = ""

	ack, err := client.SubmitAction(context.Background(), cmd)
	if ack != nil {
		t.Fatalf("expected nil ack, got %#v", ack)
	}
	requireErrorCode(t, err, CodeValidation)
	if len(pub.calls) != 0 {
		t.Fatalf("expected zero publish calls, got %d", len(pub.calls))
	}
}

/*
TC-CLIENT-SEND-003
Type: Negative
Title: SubmitAction converts publish failure to public error
Summary:
Verifies that SubmitAction converts runtime publish failures returned by the
transport publisher into public typed errors at the facade boundary.

Validates:
  - publish failure returns nil ack
  - returned error code is CodePublishFailed
  - op and subject are preserved in converted public error
*/
func TestSubmitActionReturnsPublishFailure(t *testing.T) {
	client, pub := newSendTestClient(t)
	pub.err = &runtimeerr.Error{
		Code:      runtimeerr.CodePublishFailed,
		Op:        "submit_action_publish",
		Subject:   "cmd.action.vyos.trace",
		Message:   "publish failed",
		Retryable: true,
	}

	ack, err := client.SubmitAction(context.Background(), validSendActionCommand())
	if ack != nil {
		t.Fatalf("expected nil ack, got %#v", ack)
	}
	got := requireErrorCode(t, err, CodePublishFailed)
	if got.Op != "submit_action_publish" {
		t.Fatalf("expected op %q, got %q", "submit_action_publish", got.Op)
	}
	if got.Subject != "cmd.action.vyos.trace" {
		t.Fatalf("expected subject %q, got %q", "cmd.action.vyos.trace", got.Subject)
	}
}

/*
TC-CLIENT-SEND-004
Type: Positive
Title: PublishResult publishes encoded result envelope
Summary:
Verifies that PublishResult validates and publishes to the configured result
subject while preserving envelope fields in the encoded payload.

Validates:
  - publish succeeds and is called once
  - publish metadata uses publish_result operation and result kind
  - encoded payload preserves result envelope fields
*/
func TestPublishResultPublishesEnvelope(t *testing.T) {
	client, pub := newSendTestClient(t)
	msg := validSendResultEnvelope()

	if err := client.PublishResult(context.Background(), msg); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}

	call := pub.calls[0]
	if call.op != "publish_result" {
		t.Fatalf("expected publish op %q, got %q", "publish_result", call.op)
	}
	if call.kind != "result" {
		t.Fatalf("expected publish kind %q, got %q", "result", call.kind)
	}
	if call.subject != "result.vyos" {
		t.Fatalf("expected publish subject %q, got %q", "result.vyos", call.subject)
	}

	var decoded ResultEnvelope
	if err := json.Unmarshal(call.payload, &decoded); err != nil {
		t.Fatalf("failed to decode published result payload: %v", err)
	}
	if decoded.RPCID != msg.RPCID || decoded.Target != msg.Target || decoded.Result != msg.Result {
		t.Fatalf("unexpected result identity fields: %+v", decoded)
	}
	if decoded.CommandType != msg.CommandType || decoded.UUID != msg.UUID || decoded.Action != msg.Action {
		t.Fatalf("unexpected result optional fields: %+v", decoded)
	}
	if string(decoded.Payload) != string(msg.Payload) {
		t.Fatalf("expected payload %s, got %s", string(msg.Payload), string(decoded.Payload))
	}
	if !decoded.Timestamp.Equal(msg.Timestamp) {
		t.Fatalf("expected timestamp %v, got %v", msg.Timestamp, decoded.Timestamp)
	}
}

/*
TC-CLIENT-SEND-005
Type: Negative
Title: PublishResult rejects invalid envelope before publishing
Summary:
Verifies that PublishResult performs facade validation and rejects invalid
result envelopes before invoking transport publish behavior.

Validates:
  - invalid result envelope returns CodeValidation
  - publish is not called on validation failure
*/
func TestPublishResultRejectsInvalidEnvelopeBeforePublish(t *testing.T) {
	client, pub := newSendTestClient(t)
	msg := validSendResultEnvelope()
	msg.RPCID = ""

	err := client.PublishResult(context.Background(), msg)
	requireErrorCode(t, err, CodeValidation)
	if len(pub.calls) != 0 {
		t.Fatalf("expected zero publish calls, got %d", len(pub.calls))
	}
}

/*
TC-CLIENT-SEND-006
Type: Positive
Title: PublishStatus publishes envelope and preserves optional rpc_id
Summary:
Verifies that PublishStatus publishes a valid status envelope including optional
rpc_id and preserves status fields in the encoded payload.

Validates:
  - publish succeeds to configured status subject
  - encoded payload preserves rpc_id and status fields
  - publish metadata uses publish_status operation and status kind
*/
func TestPublishStatusPublishesEnvelopeWithOptionalRPCID(t *testing.T) {
	client, pub := newSendTestClient(t)
	msg := validSendStatusEnvelope()

	if err := client.PublishStatus(context.Background(), msg); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}

	call := pub.calls[0]
	if call.op != "publish_status" {
		t.Fatalf("expected publish op %q, got %q", "publish_status", call.op)
	}
	if call.kind != "status" {
		t.Fatalf("expected publish kind %q, got %q", "status", call.kind)
	}
	if call.subject != "status.vyos" {
		t.Fatalf("expected publish subject %q, got %q", "status.vyos", call.subject)
	}

	var decoded StatusEnvelope
	if err := json.Unmarshal(call.payload, &decoded); err != nil {
		t.Fatalf("failed to decode published status payload: %v", err)
	}
	if decoded.RPCID != msg.RPCID {
		t.Fatalf("expected rpc_id %q, got %q", msg.RPCID, decoded.RPCID)
	}
	if decoded.Target != msg.Target || decoded.Status != msg.Status || decoded.Stage != msg.Stage {
		t.Fatalf("unexpected status fields: %+v", decoded)
	}
	if string(decoded.Payload) != string(msg.Payload) {
		t.Fatalf("expected payload %s, got %s", string(msg.Payload), string(decoded.Payload))
	}
	if !decoded.Timestamp.Equal(msg.Timestamp) {
		t.Fatalf("expected timestamp %v, got %v", msg.Timestamp, decoded.Timestamp)
	}
}

/*
TC-CLIENT-SEND-007
Type: Positive
Title: PublishStatus allows missing rpc_id
Summary:
Verifies that PublishStatus accepts status envelopes with an empty rpc_id and
still publishes successfully on the configured status subject.

Validates:
  - empty rpc_id is accepted
  - publish is called exactly once
  - encoded payload preserves empty rpc_id
*/
func TestPublishStatusAllowsMissingRPCID(t *testing.T) {
	client, pub := newSendTestClient(t)
	msg := validSendStatusEnvelope()
	msg.RPCID = ""

	if err := client.PublishStatus(context.Background(), msg); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}

	var decoded StatusEnvelope
	if err := json.Unmarshal(pub.calls[0].payload, &decoded); err != nil {
		t.Fatalf("failed to decode published status payload: %v", err)
	}
	if decoded.RPCID != "" {
		t.Fatalf("expected empty rpc_id, got %q", decoded.RPCID)
	}
}

/*
TC-CLIENT-SEND-008
Type: Negative
Title: PublishStatus rejects invalid envelope before publishing
Summary:
Verifies that PublishStatus rejects invalid status envelopes through facade
validation and avoids transport publish side effects on failure.

Validates:
  - invalid status envelope returns CodeValidation
  - publish is not called on validation failure
*/
func TestPublishStatusRejectsInvalidEnvelopeBeforePublish(t *testing.T) {
	client, pub := newSendTestClient(t)
	msg := validSendStatusEnvelope()
	msg.Status = ""

	err := client.PublishStatus(context.Background(), msg)
	requireErrorCode(t, err, CodeValidation)
	if len(pub.calls) != 0 {
		t.Fatalf("expected zero publish calls, got %d", len(pub.calls))
	}
}

/*
TC-CLIENT-SEND-009
Type: Positive
Title: SubmitConfigure stores desired config, publishes notification, and returns ack
Summary:
Verifies that SubmitConfigure preserves store-then-notify behavior, builds a
configure notification from stored metadata, and returns a full submission ack.

Validates:
  - desired config is stored once with preserved fields
  - configure notification is published once with stored bucket and key
  - returned ack contains configure subject and stored KV metadata
*/
func TestSubmitConfigureStoresDesiredConfigPublishesNotificationAndReturnsAck(t *testing.T) {
	client, pub := newSendTestClient(t)
	stub := &fakeStoreDesiredConfig{
		stored: &StoredDesiredConfig{
			Record:    DesiredConfigRecord{},
			Bucket:    "cfg_desired",
			Key:       "desired.vyos",
			Revision:  42,
			CreatedAt: sendFixedNow(),
		},
	}
	stub.install(client)

	cmd := validSendConfigureCommand()
	ack, err := client.SubmitConfigure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ack == nil {
		t.Fatal("expected non-nil ack")
	}
	if !ack.Accepted {
		t.Fatal("expected ack.Accepted=true")
	}
	if ack.RPCID != cmd.RPCID || ack.Target != cmd.Target {
		t.Fatalf("unexpected ack identity fields: %+v", ack)
	}
	if ack.Subject != "cmd.configure.vyos" {
		t.Fatalf("expected ack subject %q, got %q", "cmd.configure.vyos", ack.Subject)
	}
	if ack.KVBucket != "cfg_desired" || ack.KVKey != "desired.vyos" || ack.KVRevision != 42 {
		t.Fatalf("unexpected ack KV metadata: %+v", ack)
	}

	if len(stub.calls) != 1 {
		t.Fatalf("expected one store call, got %d", len(stub.calls))
	}
	storedRec := stub.calls[0]
	if storedRec.RPCID != cmd.RPCID || storedRec.Target != cmd.Target || storedRec.UUID != cmd.UUID {
		t.Fatalf("unexpected stored record identity fields: %+v", storedRec)
	}
	if string(storedRec.Payload) != string(cmd.Payload) {
		t.Fatalf("expected stored payload %s, got %s", string(cmd.Payload), string(storedRec.Payload))
	}

	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}
	call := pub.calls[0]
	if call.op != "submit_configure_publish_notification" {
		t.Fatalf("expected publish op %q, got %q", "submit_configure_publish_notification", call.op)
	}
	if call.kind != "configure" {
		t.Fatalf("expected publish kind %q, got %q", "configure", call.kind)
	}
	if call.subject != "cmd.configure.vyos" {
		t.Fatalf("expected publish subject %q, got %q", "cmd.configure.vyos", call.subject)
	}

	var notification ConfigureNotification
	if err := json.Unmarshal(call.payload, &notification); err != nil {
		t.Fatalf("failed to decode configure notification payload: %v", err)
	}
	if notification.RPCID != cmd.RPCID || notification.Target != cmd.Target || notification.UUID != cmd.UUID {
		t.Fatalf("unexpected notification identity fields: %+v", notification)
	}
	if notification.KVBucket != "cfg_desired" || notification.KVKey != "desired.vyos" {
		t.Fatalf("unexpected notification KV metadata: %+v", notification)
	}
	if notification.CommandType != "configure" {
		t.Fatalf("expected command_type %q, got %q", "configure", notification.CommandType)
	}
}

/*
TC-CLIENT-SEND-010
Type: Negative
Title: SubmitConfigure returns store failure without publishing
Summary:
Verifies that SubmitConfigure returns a KV store failure from the facade and
does not publish configure notifications when storage fails.

Validates:
  - store failure returns CodeKVStoreFailed
  - ack is nil on store failure
  - publish is not called
*/
func TestSubmitConfigureReturnsStoreFailureWithoutPublishing(t *testing.T) {
	client, pub := newSendTestClient(t)
	stub := &fakeStoreDesiredConfig{
		err: &Error{
			Code:      CodeKVStoreFailed,
			Op:        "submit_configure_store_desired",
			Message:   "failed to store desired config",
			Retryable: true,
		},
	}
	stub.install(client)

	ack, err := client.SubmitConfigure(context.Background(), validSendConfigureCommand())
	if ack != nil {
		t.Fatalf("expected nil ack, got %#v", ack)
	}
	requireErrorCode(t, err, CodeKVStoreFailed)
	if len(pub.calls) != 0 {
		t.Fatalf("expected zero publish calls, got %d", len(pub.calls))
	}
}

/*
TC-CLIENT-SEND-011
Type: Negative
Title: SubmitConfigure returns publish failure after successful store
Summary:
Verifies that SubmitConfigure still performs store first, but returns a public
publish failure when configure notification publish fails.

Validates:
  - store is called exactly once
  - publish is called once after successful store
  - ack is nil and error code is CodePublishFailed
*/
func TestSubmitConfigureReturnsPublishFailureAfterStore(t *testing.T) {
	client, pub := newSendTestClient(t)
	stub := &fakeStoreDesiredConfig{
		stored: &StoredDesiredConfig{
			Record:    DesiredConfigRecord{},
			Bucket:    "cfg_desired",
			Key:       "desired.vyos",
			Revision:  99,
			CreatedAt: sendFixedNow(),
		},
	}
	stub.install(client)
	pub.err = &runtimeerr.Error{
		Code:      runtimeerr.CodePublishFailed,
		Op:        "submit_configure_publish_notification",
		Subject:   "cmd.configure.vyos",
		Message:   "publish failed",
		Retryable: true,
	}

	ack, err := client.SubmitConfigure(context.Background(), validSendConfigureCommand())
	if ack != nil {
		t.Fatalf("expected nil ack, got %#v", ack)
	}
	requireErrorCode(t, err, CodePublishFailed)
	if len(stub.calls) != 1 {
		t.Fatalf("expected one store call, got %d", len(stub.calls))
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}
}

/*
TC-CLIENT-SEND-012
Type: Negative
Title: Public send APIs reject nil and canceled contexts
Summary:
Verifies that all four public send/publish APIs fail fast on nil and canceled
contexts, and do not trigger store or publish side effects.

Validates:
  - nil context returns CodeValidation for each public send API
  - canceled context returns CodeValidation for each public send API
  - no publish calls and no configure-store calls occur on invalid contexts
*/
func TestPublicSendAPIsRejectNilAndCanceledContexts(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client, context.Context) error
	}{
		{
			name: "SubmitConfigure",
			call: func(client *Client, ctx context.Context) error {
				ack, err := client.SubmitConfigure(ctx, validSendConfigureCommand())
				if ack != nil {
					t.Fatalf("expected nil ack, got %#v", ack)
				}
				return err
			},
		},
		{
			name: "SubmitAction",
			call: func(client *Client, ctx context.Context) error {
				ack, err := client.SubmitAction(ctx, validSendActionCommand())
				if ack != nil {
					t.Fatalf("expected nil ack, got %#v", ack)
				}
				return err
			},
		},
		{
			name: "PublishResult",
			call: func(client *Client, ctx context.Context) error {
				return client.PublishResult(ctx, validSendResultEnvelope())
			},
		},
		{
			name: "PublishStatus",
			call: func(client *Client, ctx context.Context) error {
				return client.PublishStatus(ctx, validSendStatusEnvelope())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, pub := newSendTestClient(t)
			stub := &fakeStoreDesiredConfig{
				stored: &StoredDesiredConfig{
					Record:    DesiredConfigRecord{},
					Bucket:    "cfg_desired",
					Key:       "desired.vyos",
					Revision:  1,
					CreatedAt: sendFixedNow(),
				},
			}
			stub.install(client)

			err := tc.call(client, nil)
			requireErrorCode(t, err, CodeValidation)
			if len(pub.calls) != 0 {
				t.Fatalf("expected zero publish calls for nil context, got %d", len(pub.calls))
			}
			if len(stub.calls) != 0 {
				t.Fatalf("expected zero store calls for nil context, got %d", len(stub.calls))
			}

			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			err = tc.call(client, canceled)
			requireErrorCode(t, err, CodeValidation)
			if len(pub.calls) != 0 {
				t.Fatalf("expected zero publish calls for canceled context, got %d", len(pub.calls))
			}
			if len(stub.calls) != 0 {
				t.Fatalf("expected zero store calls for canceled context, got %d", len(stub.calls))
			}
		})
	}
}
