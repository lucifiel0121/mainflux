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

func (as *adapterService) Unsubscribe(addr *net.UDPAddr, msg *gocoap.Message) error {
	id := fmt.Sprintf("%s:%d-%x", addr.IP, addr.Port, msg.Token)
	obs := as.Subs[id]
	obs.Sub.Unsubscribe()
	close(obs.MsgCh)
	return nil
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

func (as *adapterService) Subscribe(chanID, clientID string, ch chan mainflux.RawMessage) error {
	sub, err := as.pubsub.Subscribe(chanID, ch)
	if err != nil {
		return ErrFailedSubscription
	}
	as.Subs[clientID] = nats.Observer{
		Sub:   sub,
		MsgCh: ch,
	}
	return nil
}

func (as *adapterService) serve(conn *net.UDPConn, data []byte, addr *net.UDPAddr, rh gocoap.Handler) {

	msg, err := gocoap.ParseMessage(data)
	if err != nil {
		return
	}
	var respMsg *gocoap.Message
	switch msg.Type {
	case gocoap.Reset:
		as.Unsubscribe(addr, &msg)
		respMsg = &msg
		respMsg.Type = gocoap.Acknowledgement
	default:
		respMsg = rh.ServeCOAP(conn, addr, &msg)
	}
	if respMsg != nil {
		gocoap.Transmit(conn, addr, *respMsg)
	}
}
