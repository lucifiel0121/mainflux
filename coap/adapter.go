package coap

import (
	"errors"
	"fmt"
	"net"
	"sync"

	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap/nats"
	broker "github.com/nats-io/go-nats"
)

const (
	key       string = "key"
	channel   string = "id"
	protocol  string = "coap"
	maxPktLen int    = 1500
	network          = "udp"
)

var (
	errBadRequest = errors.New("bad request")
	errBadOption  = errors.New("bad option")
)

// AdapterService struct represents CoAP adapter service implementation.
type adapterService struct {
	pubsub nats.Service
	Subs   map[string]nats.Observer
	mu     sync.Mutex
}

// New creates new CoAP adapter service struct.
func New(pubsub nats.Service) Service {
	return &adapterService{
		pubsub: pubsub,
		Subs:   make(map[string]nats.Observer),
		mu:     sync.Mutex{},
	}
}

func (svc *adapterService) Unsubscribe(addr *net.UDPAddr, msg *gocoap.Message) error {
	id := fmt.Sprintf("%s:%d-%x", addr.IP, addr.Port, msg.Token)
	obs, ok := svc.Subs[id]
	if !ok {
		return nil
	}
	err := obs.Sub.Unsubscribe()
	if err != nil {
		return err
	}
	delete(svc.Subs, id)
	close(obs.MsgCh)
	return nil
}

func (svc *adapterService) Publish(msg mainflux.RawMessage) error {
	if err := svc.pubsub.Publish(msg); err != nil {
		switch err {
		case broker.ErrConnectionClosed, broker.ErrInvalidConnection:
			return ErrFailedConnection
		default:
			return ErrFailedMessagePublish
		}
	}
	return nil
}

func (svc *adapterService) Subscribe(chanID, clientID string, ch chan mainflux.RawMessage) error {
	sub, err := svc.pubsub.Subscribe(chanID, ch)
	if err != nil {
		return ErrFailedSubscription
	}
	svc.Subs[clientID] = nats.Observer{
		Sub:   sub,
		MsgCh: ch,
	}
	return nil
}
