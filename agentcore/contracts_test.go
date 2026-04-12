package agentcore

import (
	"encoding/json"
	"testing"
	"time"
)

func contractTestTime() time.Time {
	return time.Unix(1700000000, 0).UTC()
}

/*
TC-CONTRACT-001
Type: Positive
Title: ConfigureCommand survives JSON round-trip
Summary:
Verifies basic JSON sanity for the public configure submission model.

Validates:
  - marshal succeeds
  - unmarshal succeeds
  - key public fields survive round-trip intact
*/
func TestConfigureCommandJSONRoundTrip(t *testing.T) {
	want := ConfigureCommand{
		Version:   "1.0",
		RPCID:     "rpc-config-1",
		Target:    "vyos",
		UUID:      "cfg-001",
		Payload:   json.RawMessage(`{"hostname":"router-1"}`),
		Timestamp: contractTestTime(),
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got ConfigureCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Version != want.Version {
		t.Fatalf("expected Version %q, got %q", want.Version, got.Version)
	}
	if got.RPCID != want.RPCID {
		t.Fatalf("expected RPCID %q, got %q", want.RPCID, got.RPCID)
	}
	if got.Target != want.Target {
		t.Fatalf("expected Target %q, got %q", want.Target, got.Target)
	}
	if got.UUID != want.UUID {
		t.Fatalf("expected UUID %q, got %q", want.UUID, got.UUID)
	}
	if string(got.Payload) != string(want.Payload) {
		t.Fatalf("expected Payload %s, got %s", string(want.Payload), string(got.Payload))
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Fatalf("expected Timestamp %v, got %v", want.Timestamp, got.Timestamp)
	}
}

/*
TC-CONTRACT-002
Type: Positive
Title: ActionCommand survives JSON round-trip
Summary:
Verifies basic JSON sanity for the public action submission model.

Validates:
  - marshal succeeds
  - unmarshal succeeds
  - command metadata and payload survive round-trip intact
*/
func TestActionCommandJSONRoundTrip(t *testing.T) {
	want := ActionCommand{
		Version:     "1.0",
		RPCID:       "rpc-action-1",
		Target:      "vyos",
		CommandType: "action",
		Action:      "trace",
		Payload:     json.RawMessage(`{"destination":"8.8.8.8"}`),
		Timestamp:   contractTestTime(),
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got ActionCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Version != want.Version {
		t.Fatalf("expected Version %q, got %q", want.Version, got.Version)
	}
	if got.RPCID != want.RPCID {
		t.Fatalf("expected RPCID %q, got %q", want.RPCID, got.RPCID)
	}
	if got.Target != want.Target {
		t.Fatalf("expected Target %q, got %q", want.Target, got.Target)
	}
	if got.CommandType != want.CommandType {
		t.Fatalf("expected CommandType %q, got %q", want.CommandType, got.CommandType)
	}
	if got.Action != want.Action {
		t.Fatalf("expected Action %q, got %q", want.Action, got.Action)
	}
	if string(got.Payload) != string(want.Payload) {
		t.Fatalf("expected Payload %s, got %s", string(want.Payload), string(got.Payload))
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Fatalf("expected Timestamp %v, got %v", want.Timestamp, got.Timestamp)
	}
}

/*
TC-CONTRACT-003
Type: Positive
Title: ResultEnvelope survives JSON round-trip
Summary:
Verifies basic JSON sanity for the public result model.

Validates:
  - marshal succeeds
  - unmarshal succeeds
  - result correlation fields survive round-trip intact
*/
func TestResultEnvelopeJSONRoundTrip(t *testing.T) {
	want := ResultEnvelope{
		Version:     "1.0",
		RPCID:       "rpc-result-1",
		Target:      "vyos",
		CommandType: "configure",
		UUID:        "cfg-001",
		Action:      "trace",
		Result:      "success",
		Message:     "applied",
		ErrorCode:   "",
		Payload:     json.RawMessage(`{"applied":true}`),
		Timestamp:   contractTestTime(),
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got ResultEnvelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Version != want.Version {
		t.Fatalf("expected Version %q, got %q", want.Version, got.Version)
	}
	if got.RPCID != want.RPCID {
		t.Fatalf("expected RPCID %q, got %q", want.RPCID, got.RPCID)
	}
	if got.Target != want.Target {
		t.Fatalf("expected Target %q, got %q", want.Target, got.Target)
	}
	if got.CommandType != want.CommandType {
		t.Fatalf("expected CommandType %q, got %q", want.CommandType, got.CommandType)
	}
	if got.UUID != want.UUID {
		t.Fatalf("expected UUID %q, got %q", want.UUID, got.UUID)
	}
	if got.Action != want.Action {
		t.Fatalf("expected Action %q, got %q", want.Action, got.Action)
	}
	if got.Result != want.Result {
		t.Fatalf("expected Result %q, got %q", want.Result, got.Result)
	}
	if got.Message != want.Message {
		t.Fatalf("expected Message %q, got %q", want.Message, got.Message)
	}
	if string(got.Payload) != string(want.Payload) {
		t.Fatalf("expected Payload %s, got %s", string(want.Payload), string(got.Payload))
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Fatalf("expected Timestamp %v, got %v", want.Timestamp, got.Timestamp)
	}
}

/*
TC-CONTRACT-004
Type: Positive
Title: StatusEnvelope survives JSON round-trip
Summary:
Verifies basic JSON sanity for the public status model.

Validates:
  - marshal succeeds
  - unmarshal succeeds
  - status fields and payload survive round-trip intact
*/
func TestStatusEnvelopeJSONRoundTrip(t *testing.T) {
	want := StatusEnvelope{
		Version:   "1.0",
		RPCID:     "rpc-status-1",
		Target:    "vyos",
		UUID:      "cfg-001",
		Status:    "running",
		Stage:     "startup",
		Message:   "agent ready",
		Payload:   json.RawMessage(`{"ready":true}`),
		Timestamp: contractTestTime(),
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got StatusEnvelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Version != want.Version {
		t.Fatalf("expected Version %q, got %q", want.Version, got.Version)
	}
	if got.RPCID != want.RPCID {
		t.Fatalf("expected RPCID %q, got %q", want.RPCID, got.RPCID)
	}
	if got.Target != want.Target {
		t.Fatalf("expected Target %q, got %q", want.Target, got.Target)
	}
	if got.UUID != want.UUID {
		t.Fatalf("expected UUID %q, got %q", want.UUID, got.UUID)
	}
	if got.Status != want.Status {
		t.Fatalf("expected Status %q, got %q", want.Status, got.Status)
	}
	if got.Stage != want.Stage {
		t.Fatalf("expected Stage %q, got %q", want.Stage, got.Stage)
	}
	if got.Message != want.Message {
		t.Fatalf("expected Message %q, got %q", want.Message, got.Message)
	}
	if string(got.Payload) != string(want.Payload) {
		t.Fatalf("expected Payload %s, got %s", string(want.Payload), string(got.Payload))
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Fatalf("expected Timestamp %v, got %v", want.Timestamp, got.Timestamp)
	}
}

/*
TC-CONTRACT-005
Type: Positive
Title: HealthSnapshot survives JSON round-trip
Summary:
Verifies basic JSON sanity for the public read-only health model.

Validates:
  - marshal succeeds
  - unmarshal succeeds
  - state and health counters survive round-trip intact
*/
func TestHealthSnapshotJSONRoundTrip(t *testing.T) {
	want := HealthSnapshot{
		State:                   StateConnected,
		ConnectedURL:            "nats://localhost:4222",
		JetStreamReady:          true,
		KVReady:                 true,
		RegisteredSubscriptions: 3,
		ActiveSubscriptions:     2,
		LastError:               "none",
	}

	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got HealthSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.State != want.State {
		t.Fatalf("expected State %q, got %q", want.State, got.State)
	}
	if got.ConnectedURL != want.ConnectedURL {
		t.Fatalf("expected ConnectedURL %q, got %q", want.ConnectedURL, got.ConnectedURL)
	}
	if got.JetStreamReady != want.JetStreamReady {
		t.Fatalf("expected JetStreamReady %v, got %v", want.JetStreamReady, got.JetStreamReady)
	}
	if got.KVReady != want.KVReady {
		t.Fatalf("expected KVReady %v, got %v", want.KVReady, got.KVReady)
	}
	if got.RegisteredSubscriptions != want.RegisteredSubscriptions {
		t.Fatalf("expected RegisteredSubscriptions %d, got %d", want.RegisteredSubscriptions, got.RegisteredSubscriptions)
	}
	if got.ActiveSubscriptions != want.ActiveSubscriptions {
		t.Fatalf("expected ActiveSubscriptions %d, got %d", want.ActiveSubscriptions, got.ActiveSubscriptions)
	}
	if got.LastError != want.LastError {
		t.Fatalf("expected LastError %q, got %q", want.LastError, got.LastError)
	}
}

/*
TC-CONTRACT-006
Type: Negative
Title: ConfigureCommand unmarshal rejects malformed JSON
Summary:
Verifies that invalid JSON input is rejected by the standard public model decoder.

Validates:
  - malformed JSON returns a non-nil unmarshal error
*/
func TestConfigureCommandUnmarshalRejectsMalformedJSON(t *testing.T) {
	data := []byte(`{"version":"1.0","rpc_id":"rpc-1","target":"vyos",`)

	var got ConfigureCommand
	if err := json.Unmarshal(data, &got); err == nil {
		t.Fatal("expected unmarshal error for malformed JSON, got nil")
	}
}
