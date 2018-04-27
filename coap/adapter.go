package coap

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

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

// Handler is a type that handles CoAP messages.
func handlePacket(l *net.UDPConn, data []byte, u *net.UDPAddr,
	rh gocoap.Handler) {

	msg, err := gocoap.ParseMessage(data)
	if err != nil {
		return
	}

	rv := rh.ServeCOAP(l, u, &msg)
	if rv != nil {
		gocoap.Transmit(l, u, *rv)
	}
}

// ListenAndServe binds to the given address and serve requests forever.
func ListenAndServe(n, addr string, rh gocoap.Handler) error {
	uaddr, err := net.ResolveUDPAddr(n, addr)
	if err != nil {
		return err
	}

	l, err := net.ListenUDP(n, uaddr)
	if err != nil {
		return err
	}

	return serve(l, rh)
}

func serve(listener *net.UDPConn, rh gocoap.Handler) error {
	buf := make([]byte, maxPktLen)
	for {
		nr, addr, err := listener.ReadFromUDP(buf)
		fmt.Printf("received: %d, %s:%d\n", nr, addr.IP, addr.Port)
		if err != nil {
			if neterr, ok := err.(net.Error); ok && (neterr.Temporary() || neterr.Timeout()) {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return err
		}
		tmp := make([]byte, nr)
		copy(tmp, buf)
		go handlePacket(listener, tmp, addr, rh)
	}
}
