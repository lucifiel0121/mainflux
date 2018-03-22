package coap

import (
	"errors"
	"fmt"
	"net"

	mux "github.com/dereulenspiegel/coap-mux"
	gocoap "github.com/dustin/go-coap"
	"github.com/go-kit/kit/log"
	"github.com/mainflux/mainflux"
	manager "github.com/mainflux/mainflux/manager/client"
	broker "github.com/nats-io/go-nats"
)

// Service reprents actual CoAP adapter service.
type Service interface {
	Observe(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message) *gocoap.Message
	Recieve(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message) *gocoap.Message
}

const (
	token    string = "token"
	prefix   string = "src.coap."
	channel  string = "id"
	protocol string = "coap"
)

var (
	errBadRequest = errors.New("bad request")
)

// AdapterService struct represents CoAP adapter service implementation.
type AdapterService struct {
	logger log.Logger
	pub    mainflux.MessagePublisher
	nc     *broker.Conn
	mc     *manager.ManagerClient
}

// New creates new CoAP adapter service struct.
func New(logger log.Logger, pub mainflux.MessagePublisher, nc *broker.Conn, mc *manager.ManagerClient) *AdapterService {
	ca := &AdapterService{
		logger: logger,
		pub:    pub,
		nc:     nc,
		mc:     mc,
	}
	return ca
}

// Recieve method processes message and pushes it to NATS.
func (ca *AdapterService) Recieve(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message) *gocoap.Message {
	ca.logger.Log("message", fmt.Sprintf("Got message in sendMessage: path=%q: %#v from %v", m.Path(), m, a))
	var res *gocoap.Message

	if m.IsConfirmable() {
		res = &gocoap.Message{
			Type:      gocoap.Acknowledgement,
			Code:      gocoap.Content,
			MessageID: m.MessageID,
			Token:     m.Token,
			Payload:   []byte(""),
		}
		res.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
	}

	if len(m.Payload) == 0 && m.IsConfirmable() {
		res.Code = gocoap.BadRequest
		return res
	}

	cid := mux.Var(m, channel)
	publisher, err := authorize(m, res, cid, ca.mc.CanAccess)
	if err != nil {
		ca.logger.Log(err)
		return res
	}

	n := mainflux.RawMessage{
		Channel:   cid,
		Publisher: publisher,
		Protocol:  protocol,
		Payload:   m.Payload,
	}

	if err := ca.pub.Publish(n); err != nil {
		if m.IsConfirmable() {
			res.Code = gocoap.InternalServerError
		}
		return res
	}

	if m.IsConfirmable() {
		res.Code = gocoap.Changed
	}
	return res
}

// Observe method deals with CoAP observe messages.
func (ca *AdapterService) Observe(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message) *gocoap.Message {
	ca.logger.Log("message", fmt.Sprintf("Got message in ObserveMessage: path=%q: %#v from %v", m.Path(), m, a))
	var res *gocoap.Message
	if m.IsConfirmable() {
		res = &gocoap.Message{
			Type:      gocoap.Acknowledgement,
			Code:      gocoap.Content,
			MessageID: m.MessageID,
			Token:     m.Token,
			Payload:   []byte(""),
		}
		res.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
	}

	cid := mux.Var(m, channel)
	publisher, err := authorize(m, res, cid, ca.mc.CanAccess)
	if err != nil {
		ca.logger.Log(err)
		return res
	}

	if value, ok := m.Option(gocoap.Observe).(uint32); ok && value == 0 {
		ca.nc.Subscribe(cid, ca.obsHandle(l, a, m, 120, publisher))
	} else {
		// TODO Handle explicit unsubscription.
	}

	return res
}
