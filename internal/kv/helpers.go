package kv

import (
	"fmt"
	"strings"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/nats-agent-core/internal/subjects"
)

func buildDesiredConfigKey(pattern, target string) (string, error) {
	trimmedPattern := strings.TrimSpace(pattern)
	if trimmedPattern == "" {
		return "", &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "build_desired_config_key",
			Message:   "kv key pattern is required",
			Retryable: false,
		}
	}
	if strings.ContainsAny(trimmedPattern, " \t\r\n") {
		return "", &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "build_desired_config_key",
			Message:   "kv key pattern cannot contain whitespace",
			Retryable: false,
		}
	}
	if strings.Count(trimmedPattern, "%s") != 1 {
		return "", &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "build_desired_config_key",
			Message:   "kv key pattern must contain exactly one %s placeholder",
			Retryable: false,
		}
	}
	residual := strings.ReplaceAll(trimmedPattern, "%s", "")
	if strings.Contains(residual, "%") {
		return "", &agentcore.Error{
			Code:      agentcore.CodeValidation,
			Op:        "build_desired_config_key",
			Message:   "kv key pattern contains unsupported format directives",
			Retryable: false,
		}
	}
	if err := subjects.ValidateTarget(target); err != nil {
		return "", err
	}
	return fmt.Sprintf(trimmedPattern, target), nil
}

func kvStoreError(op, message string, cause error) error {
	return &agentcore.Error{
		Code:      agentcore.CodeKVStoreFailed,
		Op:        op,
		Message:   message,
		Retryable: true,
		Err:       cause,
	}
}

func kvReadError(op, message string, cause error) error {
	return &agentcore.Error{
		Code:      agentcore.CodeKVReadFailed,
		Op:        op,
		Message:   message,
		Retryable: true,
		Err:       cause,
	}
}
