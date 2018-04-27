package nats

import "github.com/mainflux/mainflux"

// Service specifies NATS service API.
type Service interface {
	mainflux.MessagePublisher
	// Subscribes to channel with specified id.
	Subscribe(string, chan mainflux.RawMessage) (Subscription, error)
}

// Subscription interface represents subscription to messaging system.
type Subscription interface {
	Unsubscribe() error
}

// Observer struct represents observer of CoAP messages.
type Observer struct {
	Sub   Subscription
	MsgCh chan mainflux.RawMessage
}
