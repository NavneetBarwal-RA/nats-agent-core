package subjects

import (
	"errors"
	"strings"
	"testing"

	"github.com/routerarchitects/nats-agent-core/agentcore"
)

func requireAgentcoreError(t *testing.T, err error, wantCode agentcore.Code, wantOp string, wantMsgPart string) *agentcore.Error {
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
TC-SUBJECTS-VALIDATE-001
Type: Positive
Title: ValidateTarget accepts a valid target token
Summary:
Verifies that target validation succeeds for a normal subject token that
matches allowed character rules.

Validates:
  - non-empty target is accepted
  - target with letters digits underscore and hyphen is accepted
*/
func TestValidateTargetAcceptsValidTarget(t *testing.T) {
	if err := ValidateTarget("vyos_1-edge"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

/*
TC-SUBJECTS-VALIDATE-002
Type: Negative
Title: ValidateTarget rejects malformed target values
Summary:
Verifies that target validation rejects malformed values that would produce
unsafe or invalid subject tokens.

Validates:
  - empty or whitespace-only target fails
  - embedded whitespace dot wildcard and unsupported characters fail
*/
func TestValidateTargetRejectsMalformedValues(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		msgPart string
	}{
		{
			name:    "empty target",
			target:  "   ",
			msgPart: "target is required",
		},
		{
			name:    "embedded whitespace",
			target:  "vyos core",
			msgPart: "target cannot contain whitespace",
		},
		{
			name:    "dot token",
			target:  "vyos.core",
			msgPart: "target cannot contain '.'",
		},
		{
			name:    "wildcard token",
			target:  "vyos*",
			msgPart: "target cannot contain wildcard tokens",
		},
		{
			name:    "full wildcard token",
			target:  "vyos>",
			msgPart: "target cannot contain wildcard tokens",
		},
		{
			name:    "unsupported character",
			target:  "vyos/core",
			msgPart: "target contains unsupported characters",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTarget(tc.target)
			requireAgentcoreError(t, err, agentcore.CodeValidation, "validate_target", tc.msgPart)
		})
	}
}

/*
TC-SUBJECTS-VALIDATE-003
Type: Positive
Title: ValidateAction accepts a valid action token
Summary:
Verifies that action validation succeeds for a normal action token using the
allowed character set.

Validates:
  - non-empty action is accepted
  - action with letters digits underscore and hyphen is accepted
*/
func TestValidateActionAcceptsValidAction(t *testing.T) {
	if err := ValidateAction("trace_route-1"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

/*
TC-SUBJECTS-VALIDATE-004
Type: Negative
Title: ValidateAction rejects malformed action values
Summary:
Verifies that action validation rejects malformed values that violate token
rules used by subject builders.

Validates:
  - empty or whitespace-only action fails
  - embedded whitespace dot wildcard and unsupported characters fail
*/
func TestValidateActionRejectsMalformedValues(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		msgPart string
	}{
		{
			name:    "empty action",
			action:  "\t",
			msgPart: "action is required",
		},
		{
			name:    "embedded whitespace",
			action:  "re boot",
			msgPart: "action cannot contain whitespace",
		},
		{
			name:    "dot token",
			action:  "re.boot",
			msgPart: "action cannot contain '.'",
		},
		{
			name:    "wildcard token",
			action:  "re*boot",
			msgPart: "action cannot contain wildcard tokens",
		},
		{
			name:    "full wildcard token",
			action:  "re>boot",
			msgPart: "action cannot contain wildcard tokens",
		},
		{
			name:    "unsupported character",
			action:  "re/boot",
			msgPart: "action contains unsupported characters",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAction(tc.action)
			requireAgentcoreError(t, err, agentcore.CodeValidation, "validate_action", tc.msgPart)
		})
	}
}

/*
TC-SUBJECTS-VALIDATE-005
Type: Positive
Title: validatePattern accepts valid subject patterns
Summary:
Verifies that internal subject pattern validation accepts valid pattern values
with the expected placeholder count.

Validates:
  - one-placeholder patterns pass when placeholders is one
  - two-placeholder action pattern passes when placeholders is two
*/
func TestValidatePatternAcceptsValidPatterns(t *testing.T) {
	tests := []struct {
		name         string
		field        string
		pattern      string
		placeholders int
	}{
		{
			name:         "configure pattern",
			field:        "configure_pattern",
			pattern:      "cmd.configure.%s",
			placeholders: 1,
		},
		{
			name:         "action pattern",
			field:        "action_pattern",
			pattern:      "cmd.action.%s.%s",
			placeholders: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validatePattern(tc.field, tc.pattern, tc.placeholders); err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

/*
TC-SUBJECTS-VALIDATE-006
Type: Negative
Title: validatePattern rejects invalid subject patterns
Summary:
Verifies that internal subject pattern validation rejects malformed patterns
that violate whitespace wildcard placeholder or formatting rules.

Validates:
  - required pattern check fails for whitespace-only values
  - wildcard and placeholder count checks fail for invalid patterns
  - unsupported format directives are rejected
*/
func TestValidatePatternRejectsInvalidPatterns(t *testing.T) {
	tests := []struct {
		name         string
		field        string
		pattern      string
		placeholders int
		msgPart      string
	}{
		{
			name:         "whitespace only pattern",
			field:        "configure_pattern",
			pattern:      "   ",
			placeholders: 1,
			msgPart:      "configure_pattern is required",
		},
		{
			name:         "pattern contains whitespace",
			field:        "configure_pattern",
			pattern:      "cmd.configure. %s",
			placeholders: 1,
			msgPart:      "configure_pattern cannot contain whitespace",
		},
		{
			name:         "pattern contains wildcard",
			field:        "status_pattern",
			pattern:      "status.*",
			placeholders: 1,
			msgPart:      "status_pattern cannot contain wildcard tokens",
		},
		{
			name:         "invalid placeholder count",
			field:        "result_pattern",
			pattern:      "result.%s.%s",
			placeholders: 1,
			msgPart:      "result_pattern placeholder count is invalid",
		},
		{
			name:         "unsupported format directive",
			field:        "health_pattern",
			pattern:      "health.%s.%d",
			placeholders: 1,
			msgPart:      "health_pattern contains unsupported format directives",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePattern(tc.field, tc.pattern, tc.placeholders)
			requireAgentcoreError(t, err, agentcore.CodeValidation, "validate_subject_pattern", tc.msgPart)
		})
	}
}
