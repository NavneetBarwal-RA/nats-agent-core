package transport

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/nats-agent-core/internal/contract"
	"github.com/routerarchitects/nats-agent-core/internal/subjects"
)

type storeCall struct {
	record agentcore.DesiredConfigRecord
}

type stubDesiredConfigStore struct {
	calls   []storeCall
	result  *agentcore.StoredDesiredConfig
	err     error
	onStore func(record agentcore.DesiredConfigRecord)
}

func (s *stubDesiredConfigStore) StoreDesiredConfig(_ context.Context, record agentcore.DesiredConfigRecord) (*agentcore.StoredDesiredConfig, error) {
	s.calls = append(s.calls, storeCall{record: record})
	if s.onStore != nil {
		s.onStore(record)
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func validTransportConfigureCommand() agentcore.ConfigureCommand {
	return agentcore.ConfigureCommand{
		Version:   "1.0",
		RPCID:     "rpc-config-1",
		Target:    "vyos",
		UUID:      "cfg-001",
		Payload:   transportRawJSON(`{"hostname":"router-1"}`),
		Timestamp: transportTestTime(),
	}
}

/*
TC-TRANSPORT-CONFIGURE-001
Type: Positive
Title: NewConfigurePaths applies defaults when KV config and clock are empty
Summary:
Verifies that configure path construction initializes default KV bucket and key
pattern values and a fallback internal clock when optional inputs are empty.

Validates:
  - default bucket and key pattern are stored
  - fallback clock is initialized
*/
func TestNewConfigurePathsAppliesDefaults(t *testing.T) {
	store := &stubDesiredConfigStore{}
	pub := &stubPublisher{}

	paths, err := NewConfigurePaths(store, pub, subjects.NewDefaultBuilder(), agentcore.KVConfig{}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if paths == nil {
		t.Fatal("expected non-nil configure paths")
	}
	if paths.kvBucket != defaultKVBucketPattern {
		t.Fatalf("expected kvBucket %q, got %q", defaultKVBucketPattern, paths.kvBucket)
	}
	if paths.kvKeyPattern != defaultKVKeyPattern {
		t.Fatalf("expected kvKeyPattern %q, got %q", defaultKVKeyPattern, paths.kvKeyPattern)
	}
	if paths.now == nil {
		t.Fatal("expected configure paths clock to be initialized")
	}
}

/*
TC-TRANSPORT-CONFIGURE-002
Type: Positive
Title: NewConfigurePaths stores custom KV config and custom clock
Summary:
Verifies that configure path construction preserves provided custom KV bucket,
custom key pattern, and custom clock dependency.

Validates:
  - custom bucket and key pattern are stored
  - custom clock is stored and callable
*/
func TestNewConfigurePathsStoresCustomKVConfigAndClock(t *testing.T) {
	store := &stubDesiredConfigStore{}
	pub := &stubPublisher{}
	fixedNow := transportTestTime().Add(10 * time.Minute)

	paths, err := NewConfigurePaths(
		store,
		pub,
		subjects.NewDefaultBuilder(),
		agentcore.KVConfig{Bucket: "custom_bucket", KeyPattern: "custom.%s"},
		func() time.Time { return fixedNow },
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if paths.kvBucket != "custom_bucket" {
		t.Fatalf("expected kvBucket %q, got %q", "custom_bucket", paths.kvBucket)
	}
	if paths.kvKeyPattern != "custom.%s" {
		t.Fatalf("expected kvKeyPattern %q, got %q", "custom.%s", paths.kvKeyPattern)
	}
	if !paths.now().Equal(fixedNow) {
		t.Fatalf("expected custom clock value %v, got %v", fixedNow, paths.now())
	}
}

/*
TC-TRANSPORT-CONFIGURE-003
Type: Negative
Title: NewConfigurePaths rejects missing required dependencies
Summary:
Verifies that configure path construction rejects missing store or publisher
dependencies before creating the helper.

Validates:
  - nil store returns CodeValidation
  - nil publisher returns CodeValidation
*/
func TestNewConfigurePathsRejectsMissingDependencies(t *testing.T) {
	builder := subjects.NewDefaultBuilder()

	_, err := NewConfigurePaths(nil, &stubPublisher{}, builder, agentcore.KVConfig{}, nil)
	requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "new_configure_paths", "desired config store is required")

	_, err = NewConfigurePaths(&stubDesiredConfigStore{}, nil, builder, agentcore.KVConfig{}, nil)
	requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "new_configure_paths", "publisher is required")
}

/*
TC-TRANSPORT-CONFIGURE-004
Type: Negative
Title: NewConfigurePaths rejects invalid subject builder dependency
Summary:
Verifies that configure path construction propagates subject-builder validation
when the builder dependency is missing.

Validates:
  - nil subject builder fails via NewPublishPaths
  - new_publish_paths op is preserved
*/
func TestNewConfigurePathsRejectsInvalidSubjectBuilder(t *testing.T) {
	_, err := NewConfigurePaths(&stubDesiredConfigStore{}, &stubPublisher{}, nil, agentcore.KVConfig{}, nil)
	requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "new_publish_paths", "subject builder is required")
}

/*
TC-TRANSPORT-CONFIGURE-005
Type: Negative
Title: SubmitConfigure rejects invalid configure command before store
Summary:
Verifies that configure submit performs transport validation first and does not
attempt KV store or notify publish when the command is invalid.

Validates:
  - invalid configure command returns CodeValidation
  - store and publish are not called on validation failure
*/
func TestSubmitConfigureRejectsInvalidCommandBeforeStore(t *testing.T) {
	store := &stubDesiredConfigStore{}
	pub := &stubPublisher{}

	paths, err := NewConfigurePaths(store, pub, subjects.NewDefaultBuilder(), agentcore.KVConfig{}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	cmd := validTransportConfigureCommand()
	cmd.UUID = ""

	ack, err := paths.SubmitConfigure(context.Background(), cmd)
	if ack != nil {
		t.Fatalf("expected nil ack, got %#v", ack)
	}
	requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "validate_configure_command", "uuid is required")
	if len(store.calls) != 0 {
		t.Fatalf("expected store not to be called, got %d calls", len(store.calls))
	}
	if len(pub.calls) != 0 {
		t.Fatalf("expected publish not to be called, got %d calls", len(pub.calls))
	}
}

/*
TC-TRANSPORT-CONFIGURE-006
Type: Negative
Title: SubmitConfigure wraps store failure and skips notify publish
Summary:
Verifies that configure submit wraps KV store failures with store-specific typed
errors and does not publish configure notification when store fails.

Validates:
  - store failure returns CodeKVStoreFailed
  - notify publish does not happen when store fails
*/
func TestSubmitConfigureWrapsStoreFailureAndSkipsNotify(t *testing.T) {
	store := &stubDesiredConfigStore{err: errors.New("kv unavailable")}
	pub := &stubPublisher{}

	paths, err := NewConfigurePaths(store, pub, subjects.NewDefaultBuilder(), agentcore.KVConfig{}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	ack, err := paths.SubmitConfigure(context.Background(), validTransportConfigureCommand())
	if ack != nil {
		t.Fatalf("expected nil ack, got %#v", ack)
	}
	requireTransportAgentcoreError(t, err, agentcore.CodeKVStoreFailed, "submit_configure_store", "failed to store desired config")
	if len(pub.calls) != 0 {
		t.Fatalf("expected publish not to be called, got %d calls", len(pub.calls))
	}
}

/*
TC-TRANSPORT-CONFIGURE-007
Type: Negative
Title: SubmitConfigure rejects nil store result and skips notify publish
Summary:
Verifies that configure submit rejects a nil stored record result from the KV
store dependency and does not publish notification.

Validates:
  - nil stored result returns CodeKVStoreFailed
  - notify publish is not attempted
*/
func TestSubmitConfigureRejectsNilStoredResultAndSkipsNotify(t *testing.T) {
	store := &stubDesiredConfigStore{result: nil}
	pub := &stubPublisher{}

	paths, err := NewConfigurePaths(store, pub, subjects.NewDefaultBuilder(), agentcore.KVConfig{}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	ack, err := paths.SubmitConfigure(context.Background(), validTransportConfigureCommand())
	if ack != nil {
		t.Fatalf("expected nil ack, got %#v", ack)
	}
	requireTransportAgentcoreError(t, err, agentcore.CodeKVStoreFailed, "submit_configure_store", "desired config store returned nil result")
	if len(pub.calls) != 0 {
		t.Fatalf("expected publish not to be called, got %d calls", len(pub.calls))
	}
}

/*
TC-TRANSPORT-CONFIGURE-008
Type: Positive
Title: SubmitConfigure uses stored bucket and key when provided
Summary:
Verifies that configure submit prefers bucket and key returned by the store and
publishes notification plus ack using those stored values.

Validates:
  - stored bucket and key are used in notification and ack
  - returned ack includes revision and configure subject
*/
func TestSubmitConfigureUsesStoredBucketAndKeyWhenProvided(t *testing.T) {
	fixedNow := transportTestTime().Add(15 * time.Minute)
	store := &stubDesiredConfigStore{
		result: &agentcore.StoredDesiredConfig{
			Record:    agentcore.DesiredConfigRecord{},
			Bucket:    "stored_bucket",
			Key:       "stored.key",
			Revision:  42,
			CreatedAt: transportTestTime(),
		},
	}
	pub := &stubPublisher{}

	paths, err := NewConfigurePaths(store, pub, subjects.NewDefaultBuilder(), agentcore.KVConfig{}, func() time.Time { return fixedNow })
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	cmd := validTransportConfigureCommand()
	ack, err := paths.SubmitConfigure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ack == nil {
		t.Fatal("expected non-nil ack")
	}
	if ack.Subject != "cmd.configure.vyos" {
		t.Fatalf("expected ack subject %q, got %q", "cmd.configure.vyos", ack.Subject)
	}
	if ack.KVBucket != "stored_bucket" {
		t.Fatalf("expected ack KVBucket %q, got %q", "stored_bucket", ack.KVBucket)
	}
	if ack.KVKey != "stored.key" {
		t.Fatalf("expected ack KVKey %q, got %q", "stored.key", ack.KVKey)
	}
	if ack.KVRevision != 42 {
		t.Fatalf("expected ack KVRevision %d, got %d", 42, ack.KVRevision)
	}
	if !ack.AcceptedAt.Equal(fixedNow) {
		t.Fatalf("expected ack AcceptedAt %v, got %v", fixedNow, ack.AcceptedAt)
	}

	if len(store.calls) != 1 {
		t.Fatalf("expected one store call, got %d", len(store.calls))
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected one publish call, got %d", len(pub.calls))
	}
	if pub.calls[0].subject != "cmd.configure.vyos" {
		t.Fatalf("expected publish subject %q, got %q", "cmd.configure.vyos", pub.calls[0].subject)
	}

	notification, err := contract.DecodeConfigureNotification(pub.calls[0].payload)
	if err != nil {
		t.Fatalf("expected decodable configure notification payload, got %v", err)
	}
	if notification.KVBucket != "stored_bucket" {
		t.Fatalf("expected notification KVBucket %q, got %q", "stored_bucket", notification.KVBucket)
	}
	if notification.KVKey != "stored.key" {
		t.Fatalf("expected notification KVKey %q, got %q", "stored.key", notification.KVKey)
	}
	if notification.CommandType != "configure" {
		t.Fatalf("expected notification CommandType %q, got %q", "configure", notification.CommandType)
	}
}

/*
TC-TRANSPORT-CONFIGURE-009
Type: Positive
Title: SubmitConfigure falls back to configured default bucket and key pattern
Summary:
Verifies that configure submit generates fallback bucket and key when store
result does not include bucket and key values.

Validates:
  - default bucket and generated key are used in notification and ack
  - fallback key follows configured key pattern
*/
func TestSubmitConfigureFallsBackToDefaultBucketAndGeneratedKey(t *testing.T) {
	store := &stubDesiredConfigStore{
		result: &agentcore.StoredDesiredConfig{
			Record:    agentcore.DesiredConfigRecord{},
			Bucket:    "",
			Key:       "",
			Revision:  7,
			CreatedAt: transportTestTime(),
		},
	}
	pub := &stubPublisher{}

	paths, err := NewConfigurePaths(
		store,
		pub,
		subjects.NewDefaultBuilder(),
		agentcore.KVConfig{Bucket: "cfg_custom", KeyPattern: "desired.custom.%s"},
		func() time.Time { return transportTestTime() },
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	ack, err := paths.SubmitConfigure(context.Background(), validTransportConfigureCommand())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if ack.KVBucket != "cfg_custom" {
		t.Fatalf("expected ack KVBucket %q, got %q", "cfg_custom", ack.KVBucket)
	}
	if ack.KVKey != "desired.custom.vyos" {
		t.Fatalf("expected ack KVKey %q, got %q", "desired.custom.vyos", ack.KVKey)
	}

	notification, err := contract.DecodeConfigureNotification(pub.calls[0].payload)
	if err != nil {
		t.Fatalf("expected decodable configure notification payload, got %v", err)
	}
	if notification.KVBucket != "cfg_custom" {
		t.Fatalf("expected notification KVBucket %q, got %q", "cfg_custom", notification.KVBucket)
	}
	if notification.KVKey != "desired.custom.vyos" {
		t.Fatalf("expected notification KVKey %q, got %q", "desired.custom.vyos", notification.KVKey)
	}
}

/*
TC-TRANSPORT-CONFIGURE-010
Type: Negative
Title: SubmitConfigure fails when fallback key pattern is invalid
Summary:
Verifies that configure submit returns key-pattern validation errors when it
must generate a fallback key from an invalid configured pattern.

Validates:
  - invalid key pattern returns CodeValidation
  - notify publish is not attempted when key generation fails
*/
func TestSubmitConfigureFailsWhenFallbackKeyPatternIsInvalid(t *testing.T) {
	store := &stubDesiredConfigStore{
		result: &agentcore.StoredDesiredConfig{
			Record:    agentcore.DesiredConfigRecord{},
			Bucket:    "",
			Key:       "",
			Revision:  1,
			CreatedAt: transportTestTime(),
		},
	}
	pub := &stubPublisher{}

	paths, err := NewConfigurePaths(
		store,
		pub,
		subjects.NewDefaultBuilder(),
		agentcore.KVConfig{Bucket: "cfg_desired", KeyPattern: "desired.%s.%d"},
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	ack, err := paths.SubmitConfigure(context.Background(), validTransportConfigureCommand())
	if ack != nil {
		t.Fatalf("expected nil ack, got %#v", ack)
	}
	requireTransportAgentcoreError(t, err, agentcore.CodeValidation, "build_kv_key", "kv key pattern contains unsupported format directives")
	if len(pub.calls) != 0 {
		t.Fatalf("expected publish not to be called, got %d calls", len(pub.calls))
	}
}

/*
TC-TRANSPORT-CONFIGURE-011
Type: Positive
Title: SubmitConfigure performs store before notify publish
Summary:
Verifies that configure flow execution preserves store-then-notify ordering by
calling the store before any notification publish.

Validates:
  - store call happens before publish call
  - both steps run on successful configure submit
*/
func TestSubmitConfigurePerformsStoreBeforeNotifyPublish(t *testing.T) {
	order := make([]string, 0, 2)
	store := &stubDesiredConfigStore{
		result: &agentcore.StoredDesiredConfig{
			Record:    agentcore.DesiredConfigRecord{},
			Bucket:    "cfg_desired",
			Key:       "desired.vyos",
			Revision:  9,
			CreatedAt: transportTestTime(),
		},
		onStore: func(_ agentcore.DesiredConfigRecord) {
			order = append(order, "store")
		},
	}
	pub := &stubPublisher{
		onPublish: func(_ string, _ []byte) {
			order = append(order, "publish")
		},
	}

	paths, err := NewConfigurePaths(store, pub, subjects.NewDefaultBuilder(), agentcore.KVConfig{}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	_, err = paths.SubmitConfigure(context.Background(), validTransportConfigureCommand())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(order) != 2 {
		t.Fatalf("expected call order length 2, got %d", len(order))
	}
	if order[0] != "store" || order[1] != "publish" {
		t.Fatalf("expected call order [store publish], got %v", order)
	}
}

/*
TC-TRANSPORT-CONFIGURE-012
Type: Positive
Title: buildKVKey builds key for valid pattern and target
Summary:
Verifies that KV key builder produces expected key output when given a valid
pattern and valid target token.

Validates:
  - desired.<target> key is produced for default pattern
*/
func TestBuildKVKeyBuildsKeyForValidPatternAndTarget(t *testing.T) {
	key, err := buildKVKey("desired.%s", "vyos")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if key != "desired.vyos" {
		t.Fatalf("expected key %q, got %q", "desired.vyos", key)
	}
}

/*
TC-TRANSPORT-CONFIGURE-013
Type: Negative
Title: buildKVKey rejects invalid pattern and target inputs
Summary:
Verifies that KV key builder rejects malformed patterns and invalid targets
before returning any key.

Validates:
  - empty pattern placeholder mismatch and unsupported directives fail
  - invalid target fails via target validator
*/
func TestBuildKVKeyRejectsInvalidPatternAndTargetInputs(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		wantOp  string
		msgPart string
	}{
		{
			name:    "empty pattern",
			pattern: "  ",
			target:  "vyos",
			wantOp:  "build_kv_key",
			msgPart: "kv key pattern is required",
		},
		{
			name:    "pattern contains whitespace",
			pattern: "desired. %s",
			target:  "vyos",
			wantOp:  "build_kv_key",
			msgPart: "kv key pattern cannot contain whitespace",
		},
		{
			name:    "placeholder mismatch",
			pattern: "desired.key",
			target:  "vyos",
			wantOp:  "build_kv_key",
			msgPart: "kv key pattern must contain exactly one %s placeholder",
		},
		{
			name:    "unsupported format directive",
			pattern: "desired.%s.%d",
			target:  "vyos",
			wantOp:  "build_kv_key",
			msgPart: "kv key pattern contains unsupported format directives",
		},
		{
			name:    "invalid target",
			pattern: "desired.%s",
			target:  "vyos core",
			wantOp:  "validate_target",
			msgPart: "target cannot contain whitespace",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildKVKey(tc.pattern, tc.target)
			requireTransportAgentcoreError(t, err, agentcore.CodeValidation, tc.wantOp, tc.msgPart)
		})
	}
}

/*
TC-TRANSPORT-CONFIGURE-014
Type: Negative
Title: SubmitConfigure returns publish failure after successful store
Summary:
Verifies that configure submit returns a publish failure when notify publish
fails after a successful store, while keeping store side effects intact.

Validates:
  - publish failure returns CodePublishFailed with publish op context
  - ack is nil when notify publish fails
  - store is called exactly once before publish failure is returned
*/
func TestSubmitConfigureReturnsPublishFailureAfterSuccessfulStore(t *testing.T) {
	order := make([]string, 0, 2)
	store := &stubDesiredConfigStore{
		result: &agentcore.StoredDesiredConfig{
			Record:    agentcore.DesiredConfigRecord{},
			Bucket:    "cfg_desired",
			Key:       "desired.vyos",
			Revision:  11,
			CreatedAt: transportTestTime(),
		},
		onStore: func(_ agentcore.DesiredConfigRecord) {
			order = append(order, "store")
		},
	}

	publishCause := errors.New("notify publish failed")
	pub := &stubPublisher{
		err: publishCause,
		onPublish: func(_ string, _ []byte) {
			order = append(order, "publish")
		},
	}

	paths, err := NewConfigurePaths(store, pub, subjects.NewDefaultBuilder(), agentcore.KVConfig{}, nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	ack, err := paths.SubmitConfigure(context.Background(), validTransportConfigureCommand())
	if ack != nil {
		t.Fatalf("expected nil ack, got %#v", ack)
	}

	got := requireTransportAgentcoreError(t, err, agentcore.CodePublishFailed, "publish_configure_notification", "publish failed")
	if got.Op == "submit_configure_store" {
		t.Fatal("expected publish failure to not be reported as store failure")
	}
	if !errors.Is(got, publishCause) {
		t.Fatal("expected wrapped publish cause to be reachable via errors.Is")
	}
	if len(store.calls) != 1 {
		t.Fatalf("expected exactly one store call, got %d", len(store.calls))
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected exactly one publish call, got %d", len(pub.calls))
	}
	if len(order) != 2 || order[0] != "store" || order[1] != "publish" {
		t.Fatalf("expected call order [store publish], got %v", order)
	}
}
