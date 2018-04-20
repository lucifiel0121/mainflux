package coap

import (
	"errors"
	"net"

	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
	broker "github.com/nats-io/go-nats"
)

// MsgHandler handles messages CoAP server recieved.
type MsgHandler func(*net.UDPConn, *net.UDPAddr, *gocoap.Message) *gocoap.Message

const (
	key       string = "key"
	channel   string = "id"
	protocol  string = "coap"
	maxPktLen int    = 1500
)

var (
	errBadRequest = errors.New("bad request")
	errBadOption  = errors.New("bad option")
)

// AdapterService struct represents CoAP adapter service implementation.
type adapterService struct {
	pubsub Service
}

// New creates new CoAP adapter service struct.
func New(pubsub Service) Service {
	return &adapterService{pubsub}
}

func (as *adapterService) Publish(msg mainflux.RawMessage) error {
	if err := as.pubsub.Publish(msg); err != nil {
		switch err {
		case broker.ErrConnectionClosed, broker.ErrInvalidConnection:
			return ErrFailedConnection
		default:
			return ErrFailedMessagePublish
		}
	}
	return nil
}

func (as *adapterService) Subscribe(chanID string, channel Channel) error {
	if err := as.pubsub.Subscribe(chanID, channel); err != nil {
		return ErrFailedSubscription
	}
	return nil
}
