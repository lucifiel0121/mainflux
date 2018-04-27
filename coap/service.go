package coap

import (
	"errors"
	"net"

	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
)

var (
	// ErrFailedMessagePublish indicates that message publishing failed.
	ErrFailedMessagePublish = errors.New("failed to publish message")

	// ErrFailedSubscription indicates that client couldn't subscribe to specified channel.
	ErrFailedSubscription = errors.New("failed to subscribe to a channel")

	// ErrFailedConnection indicates that service couldn't connect to message broker.
	ErrFailedConnection = errors.New("failed to connect to message broker")
)

// Service specifies coap service API.
type Service interface {
	mainflux.MessagePublisher
	// Subscribes to channel with specified id and adds subscription to
	// service map of subscriptions under given ID.
	Subscribe(string, string, chan mainflux.RawMessage) error
	// Unsubscribe method is used to stop observing resource.
	Unsubscribe(addr *net.UDPAddr, msg *gocoap.Message) error
	serve(conn *net.UDPConn, data []byte, addr *net.UDPAddr, rh gocoap.Handler)
}
