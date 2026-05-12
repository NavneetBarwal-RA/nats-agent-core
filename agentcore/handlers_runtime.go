package agentcore

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/routerarchitects/nats-agent-core/internal/registry"
)

// WithQueueGroup sets the optional queue group used by a subscription registration.
func WithQueueGroup(queueGroup string) SubscriptionOption {
	return func(opts *SubscriptionOptions) {
		if opts == nil {
			return
		}
		opts.QueueGroup = queueGroup
	}
}

func (c *Client) registerConfigureHandler(target string, handler ConfigureHandler, opts ...SubscriptionOption) error {
	if handler == nil {
		return validationError("register_configure_handler", "configure handler is required")
	}
	subject, err := c.subjects.ConfigureSubject(target)
	if err != nil {
		return toPublicError(err)
	}
	subOpts, err := resolveSubscriptionOptions(opts...)
	if err != nil {
		return err
	}

	snapshot, err := c.subscriptions.Add(registry.AddSpec{
		Kind:       registry.KindConfigure,
		Target:     target,
		Subject:    subject,
		QueueGroup: subOpts.QueueGroup,
		Callback:   c.bindConfigureCallback(handler),
	})
	if err != nil {
		return toPublicError(err)
	}
	c.syncSubscriptionHealth()

	c.logDebug("registered configure handler", "target", target, "subject", subject, "queue_group", subOpts.QueueGroup, "registration_id", snapshot.ID)
	if c.options.metrics != nil {
		c.options.metrics.IncSubscribe(string(registry.KindConfigure), subject, "registered")
	}

	if err := c.activateAfterRegistration(snapshot.ID, "register_configure_handler"); err != nil {
		c.rollbackRegistration(snapshot.ID, "register_configure_handler_rollback")
		return err
	}
	return nil
}

func (c *Client) registerActionHandler(target, action string, handler ActionHandler, opts ...SubscriptionOption) error {
	if handler == nil {
		return validationError("register_action_handler", "action handler is required")
	}
	subject, err := c.subjects.ActionSubject(target, action)
	if err != nil {
		return toPublicError(err)
	}
	subOpts, err := resolveSubscriptionOptions(opts...)
	if err != nil {
		return err
	}

	snapshot, err := c.subscriptions.Add(registry.AddSpec{
		Kind:       registry.KindAction,
		Target:     target,
		Action:     action,
		Subject:    subject,
		QueueGroup: subOpts.QueueGroup,
		Callback:   c.bindActionCallback(handler),
	})
	if err != nil {
		return toPublicError(err)
	}
	c.syncSubscriptionHealth()

	c.logDebug("registered action handler", "target", target, "action", action, "subject", subject, "queue_group", subOpts.QueueGroup, "registration_id", snapshot.ID)
	if c.options.metrics != nil {
		c.options.metrics.IncSubscribe(string(registry.KindAction), subject, "registered")
	}

	if err := c.activateAfterRegistration(snapshot.ID, "register_action_handler"); err != nil {
		c.rollbackRegistration(snapshot.ID, "register_action_handler_rollback")
		return err
	}
	return nil
}

func (c *Client) registerResultHandler(target string, handler ResultHandler, opts ...SubscriptionOption) error {
	if handler == nil {
		return validationError("register_result_handler", "result handler is required")
	}
	subject, err := c.subjects.ResultSubject(target)
	if err != nil {
		return toPublicError(err)
	}
	subOpts, err := resolveSubscriptionOptions(opts...)
	if err != nil {
		return err
	}

	snapshot, err := c.subscriptions.Add(registry.AddSpec{
		Kind:       registry.KindResult,
		Target:     target,
		Subject:    subject,
		QueueGroup: subOpts.QueueGroup,
		Callback:   c.bindResultCallback(handler),
	})
	if err != nil {
		return toPublicError(err)
	}
	c.syncSubscriptionHealth()

	c.logDebug("registered result handler", "target", target, "subject", subject, "queue_group", subOpts.QueueGroup, "registration_id", snapshot.ID)
	if c.options.metrics != nil {
		c.options.metrics.IncSubscribe(string(registry.KindResult), subject, "registered")
	}

	if err := c.activateAfterRegistration(snapshot.ID, "register_result_handler"); err != nil {
		c.rollbackRegistration(snapshot.ID, "register_result_handler_rollback")
		return err
	}
	return nil
}

func (c *Client) registerStatusHandler(target string, handler StatusHandler, opts ...SubscriptionOption) error {
	if handler == nil {
		return validationError("register_status_handler", "status handler is required")
	}
	subject, err := c.subjects.StatusSubject(target)
	if err != nil {
		return toPublicError(err)
	}
	subOpts, err := resolveSubscriptionOptions(opts...)
	if err != nil {
		return err
	}

	snapshot, err := c.subscriptions.Add(registry.AddSpec{
		Kind:       registry.KindStatus,
		Target:     target,
		Subject:    subject,
		QueueGroup: subOpts.QueueGroup,
		Callback:   c.bindStatusCallback(handler),
	})
	if err != nil {
		return toPublicError(err)
	}
	c.syncSubscriptionHealth()

	c.logDebug("registered status handler", "target", target, "subject", subject, "queue_group", subOpts.QueueGroup, "registration_id", snapshot.ID)
	if c.options.metrics != nil {
		c.options.metrics.IncSubscribe(string(registry.KindStatus), subject, "registered")
	}

	if err := c.activateAfterRegistration(snapshot.ID, "register_status_handler"); err != nil {
		c.rollbackRegistration(snapshot.ID, "register_status_handler_rollback")
		return err
	}
	return nil
}

func (c *Client) activateAfterRegistration(id, op string) error {
	state := c.Health().State
	if state == StateNew || state == StateConnecting {
		return nil
	}

	if err := c.activateRegisteredSubscriptionByID(id, false, op); err != nil {
		return err
	}
	c.syncSubscriptionHealth()
	return nil
}

func (c *Client) activateAllRegisteredSubscriptions(op string) error {
	records := c.subscriptions.ListActivations()
	var joined error
	for _, rec := range records {
		if err := c.activateRecord(rec, false, op); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	c.syncSubscriptionHealth()
	return joined
}

func (c *Client) restoreAllRegisteredSubscriptions() error {
	records := c.subscriptions.RestoreRecords()
	var joined error
	for _, rec := range records {
		if err := c.activateRecord(rec, true, "restore_subscriptions"); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	c.syncSubscriptionHealth()
	return joined
}

func (c *Client) activateRegisteredSubscriptionByID(id string, force bool, op string) error {
	rec, ok := c.activationRecordByID(id, force)
	if !ok {
		return nil
	}
	return c.activateRecord(rec, force, op)
}

func (c *Client) activationRecordByID(id string, force bool) (registry.ActivationRecord, bool) {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	rec, ok := c.subscriptions.GetActivationRecord(id)
	if !ok {
		return registry.ActivationRecord{}, false
	}
	if rec.Active && !force {
		return registry.ActivationRecord{}, false
	}
	return rec, true
}

func (c *Client) activateRecord(rec registry.ActivationRecord, force bool, op string) error {
	if rec.Active && !force {
		return nil
	}

	if force && rec.ActiveSub != nil {
		c.cleanupSubscription(rec.ActiveSub, op, rec.ID, rec.Subject, "failed to unsubscribe stale subscription before restore")
	}

	sub, err := c.createSubscriptionForRecord(rec, op)
	if err != nil {
		c.subMu.Lock()
		c.subscriptions.MarkInactive(rec.ID, err)
		c.subMu.Unlock()

		c.logError("subscription activation failed", "operation", op, "registration_id", rec.ID, "subject", rec.Subject, "kind", string(rec.Kind), "error", err)
		if c.options.metrics != nil {
			c.options.metrics.IncSubscribe(string(rec.Kind), rec.Subject, "failure")
		}
		return err
	}

	c.subMu.Lock()
	_, exists := c.subscriptions.GetActivationRecord(rec.ID)
	if exists {
		c.subscriptions.MarkActive(rec.ID, sub)
	}
	c.subMu.Unlock()
	if !exists {
		c.cleanupSubscription(sub, op, rec.ID, rec.Subject, "failed to cleanup active subscription after registration was removed during activation")
		return nil
	}

	c.logInfo("subscription activated", "operation", op, "registration_id", rec.ID, "subject", rec.Subject, "kind", string(rec.Kind), "queue_group", rec.QueueGroup)
	if c.options.metrics != nil {
		c.options.metrics.IncSubscribe(string(rec.Kind), rec.Subject, "success")
	}
	return nil
}

func (c *Client) createSubscriptionForRecord(rec registry.ActivationRecord, op string) (*nats.Subscription, error) {
	switch rec.Kind {
	case registry.KindConfigure:
		return c.subscribeConfigure(rec.Subject, rec.QueueGroup, rec.Callback)
	case registry.KindAction:
		return c.subscribeAction(rec.Subject, rec.QueueGroup, rec.Callback)
	case registry.KindResult:
		return c.subscribeResult(rec.Subject, rec.QueueGroup, rec.Callback)
	case registry.KindStatus:
		return c.subscribeStatus(rec.Subject, rec.QueueGroup, rec.Callback)
	default:
		return nil, &Error{
			Code:      CodeValidation,
			Op:        op,
			Subject:   rec.Subject,
			Message:   "unsupported subscription kind",
			Retryable: false,
		}
	}
}

func (c *Client) cleanupSubscription(sub *nats.Subscription, op, registrationID, subject, message string) {
	if sub == nil {
		return
	}
	if err := sub.Unsubscribe(); err != nil {
		c.logWarn(message, "operation", op, "registration_id", registrationID, "subject", subject, "error", err)
		if c.options.errorSink != nil {
			c.options.errorSink(err)
		}
	}
}

func (c *Client) deactivateAllSubscriptions(op string) error {
	c.subMu.Lock()
	handles := c.subscriptions.ClearActiveHandles()
	c.subMu.Unlock()

	var joined error
	for _, handle := range handles {
		if handle.Sub == nil {
			continue
		}
		if err := handle.Sub.Unsubscribe(); err != nil {
			joined = errors.Join(joined, &Error{
				Code:      CodeShutdown,
				Op:        op,
				Subject:   handle.Subject,
				Message:   "failed to unsubscribe active handler",
				Retryable: true,
				Err:       err,
			})
		}
	}
	c.syncSubscriptionHealth()
	return joined
}

func (c *Client) rollbackRegistration(id, op string) {
	c.subMu.Lock()
	handle, removed := c.subscriptions.Remove(id)
	c.subMu.Unlock()

	if !removed {
		c.syncSubscriptionHealth()
		return
	}

	if handle.Sub != nil {
		if err := handle.Sub.Unsubscribe(); err != nil {
			c.logWarn("failed to cleanup active subscription during registration rollback", "operation", op, "registration_id", handle.ID, "subject", handle.Subject, "error", err)
			if c.options.errorSink != nil {
				c.options.errorSink(err)
			}
		}
	}

	c.syncSubscriptionHealth()
}

func (c *Client) onSessionReconnected() {
	if !c.callbacksEnabled.Load() {
		return
	}
	c.logInfo("restoring subscriptions after reconnect")
	if err := c.restoreAllRegisteredSubscriptions(); err != nil {
		c.logError("subscription restore failed", "error", err)
		if c.options.errorSink != nil {
			c.options.errorSink(err)
		}
		return
	}
	c.logInfo("subscription restore completed")
}

func (c *Client) onSessionClosed() {
	c.callbacksEnabled.Store(false)
	c.cancelHandlerContext()

	if err := c.deactivateAllSubscriptions("session_closed"); err != nil {
		c.logWarn("failed to clear active subscriptions after session closed", "error", err)
		if c.options.errorSink != nil {
			c.options.errorSink(err)
		}
	}
}

func (c *Client) syncSubscriptionHealth() {
	if c.subscriptions == nil || c.session == nil {
		return
	}
	registered, active := c.subscriptions.Counts()
	c.session.SetSubscriptionCounts(registered, active)
}

func (c *Client) subscribeConfigure(subject, queueGroup string, callback nats.MsgHandler) (*nats.Subscription, error) {
	return c.subscribeInternal("subscribe_configure", string(registry.KindConfigure), subject, queueGroup, callback)
}

func (c *Client) subscribeAction(subject, queueGroup string, callback nats.MsgHandler) (*nats.Subscription, error) {
	return c.subscribeInternal("subscribe_action", string(registry.KindAction), subject, queueGroup, callback)
}

func (c *Client) subscribeResult(subject, queueGroup string, callback nats.MsgHandler) (*nats.Subscription, error) {
	return c.subscribeInternal("subscribe_result", string(registry.KindResult), subject, queueGroup, callback)
}

func (c *Client) subscribeStatus(subject, queueGroup string, callback nats.MsgHandler) (*nats.Subscription, error) {
	return c.subscribeInternal("subscribe_status", string(registry.KindStatus), subject, queueGroup, callback)
}

func (c *Client) subscribeInternal(op, kind, subject, queueGroup string, callback nats.MsgHandler) (*nats.Subscription, error) {
	if callback == nil {
		return nil, validationError(op, "subscription callback is required")
	}
	if strings.TrimSpace(subject) == "" {
		return nil, validationError(op, "subscription subject is required")
	}

	nc, err := c.session.Connection()
	if err != nil {
		return nil, toPublicError(err)
	}

	var sub *nats.Subscription
	if strings.TrimSpace(queueGroup) != "" {
		sub, err = nc.QueueSubscribe(subject, queueGroup, callback)
	} else {
		sub, err = nc.Subscribe(subject, callback)
	}
	if err != nil {
		return nil, &Error{
			Code:      CodeSubscribeFailed,
			Op:        op,
			Subject:   subject,
			Message:   "subscribe operation failed",
			Retryable: true,
			Err:       err,
		}
	}

	if err := nc.FlushTimeout(c.session.EffectiveConfig().Timeouts.SubscribeTimeout); err != nil {
		_ = sub.Unsubscribe()
		return nil, &Error{
			Code:      CodeSubscribeFailed,
			Op:        op,
			Subject:   subject,
			Message:   "subscribe readiness flush failed",
			Retryable: true,
			Err:       err,
		}
	}

	c.logDebug("subscription created", "kind", kind, "subject", subject, "queue_group", queueGroup)
	return sub, nil
}

func (c *Client) bindConfigureCallback(handler ConfigureHandler) nats.MsgHandler {
	return func(msg *nats.Msg) {
		if !c.callbacksEnabled.Load() {
			return
		}
		if msg == nil {
			c.logWarn("dropping nil configure message")
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindConfigure), "", "decode_failed")
			}
			return
		}
		started := time.Now()
		payload, err := decodeConfigureNotification("decode_configure_notification", msg.Data)
		if err != nil {
			c.logWarn("dropping configure message after decode/validate failure", "subject", msg.Subject, "error", err)
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindConfigure), msg.Subject, "decode_failed")
			}
			return
		}
		if err := c.callConfigureHandler(handler, payload); err != nil {
			c.logError("configure handler returned error", "subject", msg.Subject, "error", err)
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindConfigure), msg.Subject, "handler_failed")
			}
			return
		}
		if c.options.metrics != nil {
			c.options.metrics.ObservePublishLatency(string(registry.KindConfigure), msg.Subject, time.Since(started))
		}
	}
}

func (c *Client) bindActionCallback(handler ActionHandler) nats.MsgHandler {
	return func(msg *nats.Msg) {
		if !c.callbacksEnabled.Load() {
			return
		}
		if msg == nil {
			c.logWarn("dropping nil action message")
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindAction), "", "decode_failed")
			}
			return
		}
		started := time.Now()
		payload, err := decodeActionCommand("decode_action_command", msg.Data)
		if err != nil {
			c.logWarn("dropping action message after decode/validate failure", "subject", msg.Subject, "error", err)
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindAction), msg.Subject, "decode_failed")
			}
			return
		}
		if err := c.callActionHandler(handler, payload); err != nil {
			c.logError("action handler returned error", "subject", msg.Subject, "error", err)
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindAction), msg.Subject, "handler_failed")
			}
			return
		}
		if c.options.metrics != nil {
			c.options.metrics.ObservePublishLatency(string(registry.KindAction), msg.Subject, time.Since(started))
		}
	}
}

func (c *Client) bindResultCallback(handler ResultHandler) nats.MsgHandler {
	return func(msg *nats.Msg) {
		if !c.callbacksEnabled.Load() {
			return
		}
		if msg == nil {
			c.logWarn("dropping nil result message")
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindResult), "", "decode_failed")
			}
			return
		}
		started := time.Now()
		payload, err := decodeResultEnvelope("decode_result_envelope", msg.Data)
		if err != nil {
			c.logWarn("dropping result message after decode/validate failure", "subject", msg.Subject, "error", err)
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindResult), msg.Subject, "decode_failed")
			}
			return
		}
		if err := c.callResultHandler(handler, payload); err != nil {
			c.logError("result handler returned error", "subject", msg.Subject, "error", err)
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindResult), msg.Subject, "handler_failed")
			}
			return
		}
		if c.options.metrics != nil {
			c.options.metrics.ObservePublishLatency(string(registry.KindResult), msg.Subject, time.Since(started))
		}
	}
}

func (c *Client) bindStatusCallback(handler StatusHandler) nats.MsgHandler {
	return func(msg *nats.Msg) {
		if !c.callbacksEnabled.Load() {
			return
		}
		if msg == nil {
			c.logWarn("dropping nil status message")
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindStatus), "", "decode_failed")
			}
			return
		}
		started := time.Now()
		payload, err := decodeStatusEnvelope("decode_status_envelope", msg.Data)
		if err != nil {
			c.logWarn("dropping status message after decode/validate failure", "subject", msg.Subject, "error", err)
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindStatus), msg.Subject, "decode_failed")
			}
			return
		}
		if err := c.callStatusHandler(handler, payload); err != nil {
			c.logError("status handler returned error", "subject", msg.Subject, "error", err)
			if c.options.metrics != nil {
				c.options.metrics.IncSubscribe(string(registry.KindStatus), msg.Subject, "handler_failed")
			}
			return
		}
		if c.options.metrics != nil {
			c.options.metrics.ObservePublishLatency(string(registry.KindStatus), msg.Subject, time.Since(started))
		}
	}
}

func (c *Client) callConfigureHandler(handler ConfigureHandler, msg ConfigureNotification) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &Error{
				Code:      CodeSubscribeFailed,
				Op:        "dispatch_configure_handler",
				Message:   "configure handler panicked",
				Retryable: false,
				Err:       fmt.Errorf("panic: %v", recovered),
			}
		}
	}()
	return handler(c.handlerContext(), msg)
}

func (c *Client) callActionHandler(handler ActionHandler, msg ActionCommand) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &Error{
				Code:      CodeSubscribeFailed,
				Op:        "dispatch_action_handler",
				Message:   "action handler panicked",
				Retryable: false,
				Err:       fmt.Errorf("panic: %v", recovered),
			}
		}
	}()
	return handler(c.handlerContext(), msg)
}

func (c *Client) callResultHandler(handler ResultHandler, msg ResultEnvelope) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &Error{
				Code:      CodeSubscribeFailed,
				Op:        "dispatch_result_handler",
				Message:   "result handler panicked",
				Retryable: false,
				Err:       fmt.Errorf("panic: %v", recovered),
			}
		}
	}()
	return handler(c.handlerContext(), msg)
}

func (c *Client) callStatusHandler(handler StatusHandler, msg StatusEnvelope) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = &Error{
				Code:      CodeSubscribeFailed,
				Op:        "dispatch_status_handler",
				Message:   "status handler panicked",
				Retryable: false,
				Err:       fmt.Errorf("panic: %v", recovered),
			}
		}
	}()
	return handler(c.handlerContext(), msg)
}

func decodeConfigureNotification(op string, data []byte) (ConfigureNotification, error) {
	var msg ConfigureNotification
	if err := decodePayload(op, data, &msg); err != nil {
		return ConfigureNotification{}, err
	}
	if err := validateConfigureNotification(op, msg); err != nil {
		return ConfigureNotification{}, err
	}
	return msg, nil
}

func decodeActionCommand(op string, data []byte) (ActionCommand, error) {
	var msg ActionCommand
	if err := decodePayload(op, data, &msg); err != nil {
		return ActionCommand{}, err
	}
	if err := validateActionCommand(op, msg); err != nil {
		return ActionCommand{}, err
	}
	return msg, nil
}

func decodeResultEnvelope(op string, data []byte) (ResultEnvelope, error) {
	var msg ResultEnvelope
	if err := decodePayload(op, data, &msg); err != nil {
		return ResultEnvelope{}, err
	}
	if err := validateResultEnvelope(op, msg); err != nil {
		return ResultEnvelope{}, err
	}
	return msg, nil
}

func decodeStatusEnvelope(op string, data []byte) (StatusEnvelope, error) {
	var msg StatusEnvelope
	if err := decodePayload(op, data, &msg); err != nil {
		return StatusEnvelope{}, err
	}
	if err := validateStatusEnvelope(op, msg); err != nil {
		return StatusEnvelope{}, err
	}
	return msg, nil
}

func decodePayload(op string, data []byte, out any) error {
	if len(data) == 0 {
		return &Error{
			Code:      CodeValidation,
			Op:        op,
			Message:   "payload is required",
			Retryable: false,
		}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return &Error{
			Code:      CodeDecodeFailed,
			Op:        op,
			Message:   "failed to decode payload",
			Retryable: false,
			Err:       err,
		}
	}
	return nil
}

func validateConfigureNotification(op string, msg ConfigureNotification) error {
	if err := requiredString(op, "version", msg.Version); err != nil {
		return err
	}
	if err := requiredString(op, "rpc_id", msg.RPCID); err != nil {
		return err
	}
	if err := requiredString(op, "target", msg.Target); err != nil {
		return err
	}
	if err := requiredString(op, "command_type", msg.CommandType); err != nil {
		return err
	}
	if err := requiredString(op, "uuid", msg.UUID); err != nil {
		return err
	}
	if err := requiredString(op, "kv_bucket", msg.KVBucket); err != nil {
		return err
	}
	if err := requiredString(op, "kv_key", msg.KVKey); err != nil {
		return err
	}
	return requiredTimestamp(op, "timestamp", msg.Timestamp)
}

func validateActionCommand(op string, msg ActionCommand) error {
	if err := requiredString(op, "version", msg.Version); err != nil {
		return err
	}
	if err := requiredString(op, "rpc_id", msg.RPCID); err != nil {
		return err
	}
	if err := requiredString(op, "target", msg.Target); err != nil {
		return err
	}
	if err := requiredString(op, "command_type", msg.CommandType); err != nil {
		return err
	}
	if err := requiredString(op, "action", msg.Action); err != nil {
		return err
	}
	if err := requiredTimestamp(op, "timestamp", msg.Timestamp); err != nil {
		return err
	}
	return requiredJSON(op, "payload", msg.Payload)
}

func validateResultEnvelope(op string, msg ResultEnvelope) error {
	if err := requiredString(op, "version", msg.Version); err != nil {
		return err
	}
	if err := requiredString(op, "rpc_id", msg.RPCID); err != nil {
		return err
	}
	if err := requiredString(op, "target", msg.Target); err != nil {
		return err
	}
	if err := requiredString(op, "result", msg.Result); err != nil {
		return err
	}
	if err := requiredTimestamp(op, "timestamp", msg.Timestamp); err != nil {
		return err
	}
	if err := optionalString(op, "command_type", msg.CommandType); err != nil {
		return err
	}
	if err := optionalString(op, "uuid", msg.UUID); err != nil {
		return err
	}
	if err := optionalString(op, "action", msg.Action); err != nil {
		return err
	}
	if err := optionalString(op, "error_code", msg.ErrorCode); err != nil {
		return err
	}
	return optionalJSON(op, "payload", msg.Payload)
}

func validateStatusEnvelope(op string, msg StatusEnvelope) error {
	if err := requiredString(op, "version", msg.Version); err != nil {
		return err
	}
	if err := requiredString(op, "target", msg.Target); err != nil {
		return err
	}
	if err := requiredString(op, "status", msg.Status); err != nil {
		return err
	}
	if err := requiredTimestamp(op, "timestamp", msg.Timestamp); err != nil {
		return err
	}
	if err := optionalString(op, "rpc_id", msg.RPCID); err != nil {
		return err
	}
	if err := optionalString(op, "uuid", msg.UUID); err != nil {
		return err
	}
	if err := optionalString(op, "stage", msg.Stage); err != nil {
		return err
	}
	return optionalJSON(op, "payload", msg.Payload)
}

func resolveSubscriptionOptions(opts ...SubscriptionOption) (SubscriptionOptions, error) {
	var out SubscriptionOptions
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(&out)
	}
	if strings.ContainsAny(out.QueueGroup, " \t\r\n") {
		return SubscriptionOptions{}, validationError("register_handler_options", "queue group cannot contain whitespace")
	}
	return out, nil
}

func requiredString(op, field, value string) error {
	if strings.TrimSpace(value) == "" {
		return validationError(op, field+" is required")
	}
	return nil
}

func optionalString(op, field, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) == "" {
		return validationError(op, field+" cannot be whitespace")
	}
	return nil
}

func requiredTimestamp(op, field string, value time.Time) error {
	if value.IsZero() {
		return validationError(op, field+" is required")
	}
	return nil
}

func requiredJSON(op, field string, value json.RawMessage) error {
	if len(value) == 0 {
		return validationError(op, field+" is required")
	}
	if !json.Valid(value) {
		return validationError(op, field+" must contain valid JSON")
	}
	return nil
}

func optionalJSON(op, field string, value json.RawMessage) error {
	if len(value) == 0 {
		return nil
	}
	if !json.Valid(value) {
		return validationError(op, field+" must contain valid JSON")
	}
	return nil
}

func validationError(op, msg string) *Error {
	return &Error{
		Code:      CodeValidation,
		Op:        op,
		Message:   msg,
		Retryable: false,
	}
}

func (c *Client) logDebug(msg string, kv ...any) {
	if c.options.logger != nil {
		c.options.logger.Debug(msg, kv...)
	}
}

func (c *Client) logInfo(msg string, kv ...any) {
	if c.options.logger != nil {
		c.options.logger.Info(msg, kv...)
	}
}

func (c *Client) logWarn(msg string, kv ...any) {
	if c.options.logger != nil {
		c.options.logger.Warn(msg, kv...)
	}
}

func (c *Client) logError(msg string, kv ...any) {
	if c.options.logger != nil {
		c.options.logger.Error(msg, kv...)
	}
}
