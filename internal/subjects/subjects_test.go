package subjects

import (
	"testing"

	"github.com/routerarchitects/nats-agent-core/agentcore"
)

/*
TC-SUBJECTS-BUILDER-001
Type: Positive
Title: DefaultPatterns returns standard routing defaults
Summary:
Verifies that default pattern generation returns the expected target-oriented
subject formats for configure action result status and health.

Validates:
  - configure and action defaults match the standard contract
  - result status and health defaults match the standard contract
*/
func TestDefaultPatternsReturnsStandardDefaults(t *testing.T) {
	got := DefaultPatterns()

	if got.ConfigurePattern != DefaultConfigurePattern {
		t.Fatalf("expected ConfigurePattern %q, got %q", DefaultConfigurePattern, got.ConfigurePattern)
	}
	if got.ActionPattern != DefaultActionPattern {
		t.Fatalf("expected ActionPattern %q, got %q", DefaultActionPattern, got.ActionPattern)
	}
	if got.ResultPattern != DefaultResultPattern {
		t.Fatalf("expected ResultPattern %q, got %q", DefaultResultPattern, got.ResultPattern)
	}
	if got.StatusPattern != DefaultStatusPattern {
		t.Fatalf("expected StatusPattern %q, got %q", DefaultStatusPattern, got.StatusPattern)
	}
	if got.HealthPattern != DefaultHealthPattern {
		t.Fatalf("expected HealthPattern %q, got %q", DefaultHealthPattern, got.HealthPattern)
	}
}

/*
TC-SUBJECTS-BUILDER-002
Type: Positive
Title: PatternsFromConfig preserves defaults for empty config
Summary:
Verifies that pattern resolution keeps all default values when subject config
does not provide overrides.

Validates:
  - empty config returns default configure action result status and health patterns
*/
func TestPatternsFromConfigPreservesDefaultsForEmptyConfig(t *testing.T) {
	got, err := PatternsFromConfig(agentcore.SubjectConfig{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	want := DefaultPatterns()
	if got != want {
		t.Fatalf("expected patterns %+v, got %+v", want, got)
	}
}

/*
TC-SUBJECTS-BUILDER-003
Type: Positive
Title: PatternsFromConfig accepts valid overrides
Summary:
Verifies that pattern resolution accepts valid subject overrides while keeping
validation constraints enforced.

Validates:
  - configured patterns override defaults
  - valid override set passes pattern validation
*/
func TestPatternsFromConfigAcceptsValidOverrides(t *testing.T) {
	cfg := agentcore.SubjectConfig{
		ConfigurePattern: "custom.cfg.%s",
		ActionPattern:    "custom.act.%s.%s",
		ResultPattern:    "custom.result.%s",
		StatusPattern:    "custom.status.%s",
		HealthPattern:    "custom.health.%s",
	}

	got, err := PatternsFromConfig(cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if got.ConfigurePattern != cfg.ConfigurePattern {
		t.Fatalf("expected ConfigurePattern %q, got %q", cfg.ConfigurePattern, got.ConfigurePattern)
	}
	if got.ActionPattern != cfg.ActionPattern {
		t.Fatalf("expected ActionPattern %q, got %q", cfg.ActionPattern, got.ActionPattern)
	}
	if got.ResultPattern != cfg.ResultPattern {
		t.Fatalf("expected ResultPattern %q, got %q", cfg.ResultPattern, got.ResultPattern)
	}
	if got.StatusPattern != cfg.StatusPattern {
		t.Fatalf("expected StatusPattern %q, got %q", cfg.StatusPattern, got.StatusPattern)
	}
	if got.HealthPattern != cfg.HealthPattern {
		t.Fatalf("expected HealthPattern %q, got %q", cfg.HealthPattern, got.HealthPattern)
	}
}

/*
TC-SUBJECTS-BUILDER-004
Type: Positive
Title: PatternsFromConfig preserves defaults for unspecified partial overrides
Summary:
Verifies that pattern resolution applies only configured overrides and keeps
default values for every unspecified pattern field.

Validates:
  - specified configure pattern override is applied
  - unspecified patterns remain at defaults
*/
func TestPatternsFromConfigPreservesDefaultsForPartialOverrides(t *testing.T) {
	cfg := agentcore.SubjectConfig{
		ConfigurePattern: "custom.cfg.%s",
	}

	got, err := PatternsFromConfig(cfg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if got.ConfigurePattern != "custom.cfg.%s" {
		t.Fatalf("expected ConfigurePattern %q, got %q", "custom.cfg.%s", got.ConfigurePattern)
	}
	if got.ActionPattern != DefaultActionPattern {
		t.Fatalf("expected ActionPattern %q, got %q", DefaultActionPattern, got.ActionPattern)
	}
	if got.ResultPattern != DefaultResultPattern {
		t.Fatalf("expected ResultPattern %q, got %q", DefaultResultPattern, got.ResultPattern)
	}
	if got.StatusPattern != DefaultStatusPattern {
		t.Fatalf("expected StatusPattern %q, got %q", DefaultStatusPattern, got.StatusPattern)
	}
	if got.HealthPattern != DefaultHealthPattern {
		t.Fatalf("expected HealthPattern %q, got %q", DefaultHealthPattern, got.HealthPattern)
	}
}

/*
TC-SUBJECTS-BUILDER-005
Type: Negative
Title: PatternsFromConfig rejects invalid configured patterns
Summary:
Verifies that configured pattern overrides are validated and rejected when they
violate subject pattern constraints.

Validates:
  - invalid configured action placeholder count fails
  - error preserves subject-pattern validator op
*/
func TestPatternsFromConfigRejectsInvalidConfiguredPatterns(t *testing.T) {
	cfg := agentcore.SubjectConfig{
		ActionPattern: "cmd.action.%s",
	}

	_, err := PatternsFromConfig(cfg)
	requireAgentcoreError(t, err, agentcore.CodeValidation, "validate_subject_pattern", "action_pattern placeholder count is invalid")
}

/*
TC-SUBJECTS-BUILDER-006
Type: Positive
Title: NewBuilder accepts valid patterns
Summary:
Verifies that builder construction succeeds for a valid full pattern set.

Validates:
  - NewBuilder returns a non-nil builder for valid patterns
*/
func TestNewBuilderAcceptsValidPatterns(t *testing.T) {
	builder, err := NewBuilder(DefaultPatterns())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if builder == nil {
		t.Fatal("expected non-nil builder")
	}
}

/*
TC-SUBJECTS-BUILDER-007
Type: Negative
Title: NewBuilder rejects invalid patterns
Summary:
Verifies that builder construction rejects invalid pattern sets before the
builder is created.

Validates:
  - malformed configure pattern fails validation
  - error preserves subject-pattern validator op
*/
func TestNewBuilderRejectsInvalidPatterns(t *testing.T) {
	patterns := DefaultPatterns()
	patterns.ConfigurePattern = "cmd.configure.*"

	_, err := NewBuilder(patterns)
	requireAgentcoreError(t, err, agentcore.CodeValidation, "validate_subject_pattern", "configure_pattern cannot contain wildcard tokens")
}

/*
TC-SUBJECTS-BUILDER-008
Type: Positive
Title: NewDefaultBuilder provides working default builder
Summary:
Verifies that the default builder helper returns a usable builder configured
with standard defaults.

Validates:
  - default builder is non-nil
  - configure subject generation works with defaults
*/
func TestNewDefaultBuilderProvidesWorkingDefaultBuilder(t *testing.T) {
	builder := NewDefaultBuilder()
	if builder == nil {
		t.Fatal("expected non-nil builder")
	}

	subject, err := builder.ConfigureSubject("vyos")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if subject != "cmd.configure.vyos" {
		t.Fatalf("expected subject %q, got %q", "cmd.configure.vyos", subject)
	}
}

/*
TC-SUBJECTS-BUILDER-009
Type: Positive
Title: Builder methods generate expected subjects for valid inputs
Summary:
Verifies that all builder methods generate the expected subject values for
valid target and action tokens.

Validates:
  - configure action result status and health subjects are built correctly
*/
func TestBuilderMethodsGenerateExpectedSubjects(t *testing.T) {
	builder, err := NewBuilder(DefaultPatterns())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	configureSubject, err := builder.ConfigureSubject("vyos")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if configureSubject != "cmd.configure.vyos" {
		t.Fatalf("expected configure subject %q, got %q", "cmd.configure.vyos", configureSubject)
	}

	actionSubject, err := builder.ActionSubject("vyos", "trace")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if actionSubject != "cmd.action.vyos.trace" {
		t.Fatalf("expected action subject %q, got %q", "cmd.action.vyos.trace", actionSubject)
	}

	resultSubject, err := builder.ResultSubject("vyos")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resultSubject != "result.vyos" {
		t.Fatalf("expected result subject %q, got %q", "result.vyos", resultSubject)
	}

	statusSubject, err := builder.StatusSubject("vyos")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if statusSubject != "status.vyos" {
		t.Fatalf("expected status subject %q, got %q", "status.vyos", statusSubject)
	}

	healthSubject, err := builder.HealthSubject("vyos")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if healthSubject != "health.vyos" {
		t.Fatalf("expected health subject %q, got %q", "health.vyos", healthSubject)
	}
}

/*
TC-SUBJECTS-BUILDER-010
Type: Negative
Title: Builder methods reject invalid target and action inputs
Summary:
Verifies that builder methods enforce token validation and reject invalid
target or action inputs.

Validates:
  - subject methods requiring target reject invalid target
  - action subject rejects invalid action token
*/
func TestBuilderMethodsRejectInvalidTargetAndAction(t *testing.T) {
	builder, err := NewBuilder(DefaultPatterns())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	tests := []struct {
		name    string
		call    func() error
		wantOp  string
		msgPart string
	}{
		{
			name: "configure invalid target",
			call: func() error {
				_, err := builder.ConfigureSubject("vyos core")
				return err
			},
			wantOp:  "validate_target",
			msgPart: "target cannot contain whitespace",
		},
		{
			name: "result invalid target",
			call: func() error {
				_, err := builder.ResultSubject("vyos>")
				return err
			},
			wantOp:  "validate_target",
			msgPart: "target cannot contain wildcard tokens",
		},
		{
			name: "status invalid target",
			call: func() error {
				_, err := builder.StatusSubject("vyos.core")
				return err
			},
			wantOp:  "validate_target",
			msgPart: "target cannot contain '.'",
		},
		{
			name: "health invalid target",
			call: func() error {
				_, err := builder.HealthSubject("vyos/core")
				return err
			},
			wantOp:  "validate_target",
			msgPart: "target contains unsupported characters",
		},
		{
			name: "action invalid action",
			call: func() error {
				_, err := builder.ActionSubject("vyos", "re boot")
				return err
			},
			wantOp:  "validate_action",
			msgPart: "action cannot contain whitespace",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			requireAgentcoreError(t, err, agentcore.CodeValidation, tc.wantOp, tc.msgPart)
		})
	}
}
