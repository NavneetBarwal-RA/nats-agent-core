package registry

import "github.com/nats-io/nats.go"

// Kind identifies the logical receive path for a subscription registration.
type Kind string

const (
	KindConfigure Kind = "configure"
	KindAction    Kind = "action"
	KindResult    Kind = "result"
	KindStatus    Kind = "status"
)

// AddSpec contains subscription intent fields persisted in the registry.
type AddSpec struct {
	Kind       Kind
	Target     string
	Action     string
	Subject    string
	QueueGroup string
	Callback   nats.MsgHandler
}

type entry struct {
	ID         string
	Key        string
	Kind       Kind
	Target     string
	Action     string
	Subject    string
	QueueGroup string
	Callback   nats.MsgHandler

	ActiveSub         *nats.Subscription
	Active            bool
	LastActivationErr string
}

// ActivationRecord is a snapshot used by runtime activation/restore flows.
type ActivationRecord struct {
	ID         string
	Kind       Kind
	Subject    string
	QueueGroup string
	Callback   nats.MsgHandler
	ActiveSub  *nats.Subscription
	Active     bool
}

// Snapshot is a read-only copy of a registration entry.
type Snapshot struct {
	ID                string
	Kind              Kind
	Target            string
	Action            string
	Subject           string
	QueueGroup        string
	Active            bool
	LastActivationErr string
}

// ActiveHandle carries a stale runtime subscription for cleanup.
type ActiveHandle struct {
	ID      string
	Subject string
	Sub     *nats.Subscription
}
