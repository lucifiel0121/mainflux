package coap

import (
	"errors"
	"sync"

	"github.com/mainflux/mainflux"
	broker "github.com/nats-io/go-nats"
)

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
	Subs   map[string]Subscription
	mu     sync.Mutex
}

// New creates new CoAP adapter service struct.
func New(pubsub Service) Service {
	return &adapterService{
		pubsub: pubsub,
		Subs:   make(map[string]Subscription),
		mu:     sync.Mutex{},
	}
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

func (as *adapterService) Subscribe(chanID string, channel Channel) (Subscription, error) {
	sub, err := as.pubsub.Subscribe(chanID, channel)
	if err != nil {
		err = ErrFailedSubscription
	}
	return sub, err
}
