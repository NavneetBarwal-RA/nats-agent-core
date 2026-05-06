package transport

import (
	"errors"

	"github.com/routerarchitects/nats-agent-core/agentcore"
	"github.com/routerarchitects/nats-agent-core/internal/runtimeerr"
)

func toPublicError(err error) error {
	if err == nil {
		return nil
	}

	var internal *runtimeerr.Error
	if !errors.As(err, &internal) {
		return err
	}

	return &agentcore.Error{
		Code:      agentcore.Code(internal.Code),
		Op:        internal.Op,
		Subject:   internal.Subject,
		Key:       internal.Key,
		Message:   internal.Message,
		Retryable: internal.Retryable,
		Err:       internal.Err,
	}
}
