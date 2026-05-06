package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/nats-io/nats.go"
	"github.com/routerarchitects/nats-agent-core/internal/registry"
)

const (
	defaultConfigurePattern = "cmd.configure.%s"
	defaultActionPattern    = "cmd.action.%s.%s"
	defaultResultPattern    = "result.%s"
	defaultStatusPattern    = "status.%s"
)

type subscriptionSubjectPatterns struct {
	configure string
	action    string
	result    string
	status    string
}

// WithQueueGroup sets the optional queue group used by a subscription registration.
func WithQueueGroup(queueGroup string) SubscriptionOption {
	return func(opts *SubscriptionOptions) {
		if opts == nil {
			return
		}
		opts.QueueGroup = queueGroup
	}
}

func resolveSubscriptionSubjectPatterns(cfg SubjectConfig) (subscriptionSubjectPatterns, error) {
	p := subscriptionSubjectPatterns{
		configure: defaultConfigurePattern,
		action:    defaultActionPattern,
		result:    defaultResultPattern,
		status:    defaultStatusPattern,
	}
	if strings.TrimSpace(cfg.ConfigurePattern) != "" {
		p.configure = cfg.ConfigurePattern
	}
	if strings.TrimSpace(cfg.ActionPattern) != "" {
		p.action = cfg.ActionPattern
	}
	if strings.TrimSpace(cfg.ResultPattern) != "" {
		p.result = cfg.ResultPattern
	}
	if strings.TrimSpace(cfg.StatusPattern) != "" {
		p.status = cfg.StatusPattern
	}

	if err := validateSubjectPattern("validate_subject_pattern", "configure_pattern", p.configure, 1); err != nil {
		return subscriptionSubjectPatterns{}, err
	}
	if err := validateSubjectPattern("validate_subject_pattern", "action_pattern", p.action, 2); err != nil {
		return subscriptionSubjectPatterns{}, err
	}
	if err := validateSubjectPattern("validate_subject_pattern", "result_pattern", p.result, 1); err != nil {
		return subscriptionSubjectPatterns{}, err
	}
	if err := validateSubjectPattern("validate_subject_pattern", "status_pattern", p.status, 1); err != nil {
		return subscriptionSubjectPatterns{}, err
	}
	return p, nil
}

func (p subscriptionSubjectPatterns) configureSubject(target string) (string, error) {
	if err := validateSubjectToken("validate_target", "target", target, true); err != nil {
		return "", err
	}
	return fmt.Sprintf(p.configure, target), nil
}

func (p subscriptionSubjectPatterns) actionSubject(target, action string) (string, error) {
	if err := validateSubjectToken("validate_target", "target", target, true); err != nil {
		return "", err
	}
	if err := validateSubjectToken("validate_action", "action", action, true); err != nil {
		return "", err
	}
	return fmt.Sprintf(p.action, target, action), nil
}

func (p subscriptionSubjectPatterns) resultSubject(target string) (string, error) {
	if err := validateSubjectToken("validate_target", "target", target, true); err != nil {
		return "", err
	}
	return fmt.Sprintf(p.result, target), nil
}

func (p subscriptionSubjectPatterns) statusSubject(target string) (string, error) {
	if err := validateSubjectToken("validate_target", "target", target, true); err != nil {
		return "", err
	}
	return fmt.Sprintf(p.status, target), nil
}

func (c *Client) registerConfigureHandler(target string, handler ConfigureHandler, opts ...SubscriptionOption) error {
	if handler == nil {
		return validationError("register_configure_handler", "configure handler is required")
	}
	subject, err := c.subPatterns.configureSubject(target)
	if err != nil {
		return err
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

	return c.activateAfterRegistration(snapshot.ID, "register_configure_handler")
}

func (c *Client) registerActionHandler(target, action string, handler ActionHandler, opts ...SubscriptionOption) error {
	if handler == nil {
		return validationError("register_action_handler", "action handler is required")
	}
	subject, err := c.subPatterns.actionSubject(target, action)
	if err != nil {
		return err
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

	return c.activateAfterRegistration(snapshot.ID, "register_action_handler")
}

func (c *Client) registerResultHandler(target string, handler ResultHandler, opts ...SubscriptionOption) error {
	if handler == nil {
		return validationError("register_result_handler", "result handler is required")
	}
	subject, err := c.subPatterns.resultSubject(target)
	if err != nil {
		return err
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

	return c.activateAfterRegistration(snapshot.ID, "register_result_handler")
}

func (c *Client) registerStatusHandler(target string, handler StatusHandler, opts ...SubscriptionOption) error {
	if handler == nil {
		return validationError("register_status_handler", "status handler is required")
	}
	subject, err := c.subPatterns.statusSubject(target)
	if err != nil {
		return err
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

	return c.activateAfterRegistration(snapshot.ID, "register_status_handler")
}

func (c *Client) activateAfterRegistration(id, op string) error {
	state := c.Health().State
	if state == StateNew || state == StateConnecting {
		return nil
	}

	c.subMu.Lock()
	defer c.subMu.Unlock()

	if err := c.activateRegisteredSubscriptionByID(id, false, op); err != nil {
		return err
	}
	c.syncSubscriptionHealth()
	return nil
}

func (c *Client) activateAllRegisteredSubscriptions(op string) error {
	c.subMu.Lock()
	defer c.subMu.Unlock()

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
	c.subMu.Lock()
	defer c.subMu.Unlock()

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
	rec, ok := c.subscriptions.GetActivationRecord(id)
	if !ok {
		return nil
	}
	return c.activateRecord(rec, force, op)
}

func (c *Client) activateRecord(rec registry.ActivationRecord, force bool, op string) error {
	if rec.Active && !force {
		return nil
	}

	if force && rec.ActiveSub != nil {
		if err := rec.ActiveSub.Unsubscribe(); err != nil {
			c.logWarn("failed to unsubscribe stale subscription before restore", "registration_id", rec.ID, "subject", rec.Subject, "error", err)
		}
	}

	var (
		sub *nats.Subscription
		err error
	)
	switch rec.Kind {
	case registry.KindConfigure:
		sub, err = c.subscribeConfigure(rec.Subject, rec.QueueGroup, rec.Callback)
	case registry.KindAction:
		sub, err = c.subscribeAction(rec.Subject, rec.QueueGroup, rec.Callback)
	case registry.KindResult:
		sub, err = c.subscribeResult(rec.Subject, rec.QueueGroup, rec.Callback)
	case registry.KindStatus:
		sub, err = c.subscribeStatus(rec.Subject, rec.QueueGroup, rec.Callback)
	default:
		err = &Error{
			Code:      CodeValidation,
			Op:        op,
			Subject:   rec.Subject,
			Message:   "unsupported subscription kind",
			Retryable: false,
		}
	}
	if err != nil {
		c.subscriptions.MarkInactive(rec.ID, err)
		c.logError("subscription activation failed", "operation", op, "registration_id", rec.ID, "subject", rec.Subject, "kind", string(rec.Kind), "error", err)
		if c.options.metrics != nil {
			c.options.metrics.IncSubscribe(string(rec.Kind), rec.Subject, "failure")
		}
		return err
	}

	c.subscriptions.MarkActive(rec.ID, sub)
	c.logInfo("subscription activated", "operation", op, "registration_id", rec.ID, "subject", rec.Subject, "kind", string(rec.Kind), "queue_group", rec.QueueGroup)
	if c.options.metrics != nil {
		c.options.metrics.IncSubscribe(string(rec.Kind), rec.Subject, "success")
	}
	return nil
}

func (c *Client) deactivateAllSubscriptions(op string) error {
	c.subMu.Lock()
	defer c.subMu.Unlock()

	handles := c.subscriptions.ClearActiveHandles()
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

	c.logDebug("subscription created", "kind", kind, "subject", subject, "queue_group", queueGroup)
	return sub, nil
}

func (c *Client) bindConfigureCallback(handler ConfigureHandler) nats.MsgHandler {
	return func(msg *nats.Msg) {
		if !c.callbacksEnabled.Load() {
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
	return handler(context.Background(), msg)
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
	return handler(context.Background(), msg)
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
	return handler(context.Background(), msg)
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
	return handler(context.Background(), msg)
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

func validateSubjectPattern(op, field, pattern string, placeholders int) error {
	if strings.TrimSpace(pattern) == "" {
		return validationError(op, field+" is required")
	}
	if strings.ContainsAny(pattern, " \t\r\n") {
		return validationError(op, field+" cannot contain whitespace")
	}
	if strings.Contains(pattern, "*") || strings.Contains(pattern, ">") {
		return validationError(op, field+" cannot contain wildcard tokens")
	}
	count := strings.Count(pattern, "%s")
	if count != placeholders {
		return validationError(op, field+" placeholder count is invalid")
	}
	residual := strings.ReplaceAll(pattern, "%s", "")
	if strings.Contains(residual, "%") {
		return validationError(op, field+" contains unsupported format directives")
	}
	return nil
}

func validateSubjectToken(op, field, value string, required bool) error {
	if strings.TrimSpace(value) == "" {
		if required {
			return validationError(op, field+" is required")
		}
		return nil
	}
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
