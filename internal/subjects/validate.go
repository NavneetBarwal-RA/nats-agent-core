package subjects

import (
	"strings"
	"unicode"

	"github.com/routerarchitects/nats-agent-core/agentcore"
)

func validationError(op, msg string) error {
	return &agentcore.Error{
		Code:      agentcore.CodeValidation,
		Op:        op,
		Message:   msg,
		Retryable: false,
	}
}

// ValidateTarget validates target identifiers used for subject construction.
func ValidateTarget(target string) error {
	const op = "validate_target"

	if strings.TrimSpace(target) == "" {
		return validationError(op, "target is required")
	}
	return validateToken(op, "target", target)
}

// ValidateAction validates action identifiers used for subject construction.
func ValidateAction(action string) error {
	const op = "validate_action"

	if strings.TrimSpace(action) == "" {
		return validationError(op, "action is required")
	}
	return validateToken(op, "action", action)
}

func validateToken(op, field, value string) error {
	if strings.ContainsAny(value, " \t\r\n") {
		return validationError(op, field+" cannot contain whitespace")
	}
	if strings.Contains(value, ".") {
		return validationError(op, field+" cannot contain '.'")
	}
	if strings.Contains(value, "*") || strings.Contains(value, ">") {
		return validationError(op, field+" cannot contain wildcard tokens")
	}

	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			continue
		}
		return validationError(op, field+" contains unsupported characters")
	}

	return nil
}

func validatePattern(name, pattern string, placeholders int) error {
	const op = "validate_subject_pattern"

	if strings.TrimSpace(pattern) == "" {
		return validationError(op, name+" is required")
	}
	if strings.ContainsAny(pattern, " \t\r\n") {
		return validationError(op, name+" cannot contain whitespace")
	}
	if strings.Contains(pattern, "*") || strings.Contains(pattern, ">") {
		return validationError(op, name+" cannot contain wildcard tokens")
	}

	count := strings.Count(pattern, "%s")
	if count != placeholders {
		return validationError(op, name+" placeholder count is invalid")
	}

	residual := strings.ReplaceAll(pattern, "%s", "")
	if strings.Contains(residual, "%") {
		return validationError(op, name+" contains unsupported format directives")
	}

	return nil
}

