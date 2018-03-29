package coap

import (
	"errors"
	"fmt"
	"net"

	mux "github.com/dereulenspiegel/coap-mux"
	gocoap "github.com/dustin/go-coap"
	"github.com/go-kit/kit/log"
	"github.com/mainflux/mainflux"
	broker "github.com/nats-io/go-nats"
)

// PubSub interface is used to access to NATS pub-sub model.
type PubSub interface {
	Publish(mainflux.RawMessage) error
	Subscribe(string, broker.MsgHandler) (*broker.Subscription, error)
}

// MsgHandler handles messages CoAP server recieved.
type MsgHandler func(*net.UDPConn, *net.UDPAddr, *gocoap.Message) *gocoap.Message

// Service reprents actual CoAP adapter service.
type Service interface {
	Receive(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message
	Observe(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message
}

const (
	key       string = "key"
	channel   string = "id"
	protocol  string = "coap"
	maxPktLen int    = 1500
)

var (
	errBadRequest = errors.New("bad request")
)

// AdapterService struct represents CoAP adapter service implementation.
type AdapterService struct {
	logger log.Logger
	pubSub PubSub
	auth   AuthProvider
}

// New creates new CoAP adapter service struct.
func New(logger log.Logger, pubSub PubSub, auth AuthProvider) Service {
	ca := &AdapterService{
		logger: logger,
		pubSub: pubSub,
		auth:   auth,
	}
	return ca
}

// Receive method processes message and pushes it to NATS.
func (ca *AdapterService) Receive(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
	ca.logger.Log("message", fmt.Sprintf("Got message in Receive: path=%q: %#v from %v", msg.Path(), msg, addr))
	var res *gocoap.Message

	if msg.IsConfirmable() {
		res = &gocoap.Message{
			Type:      gocoap.Acknowledgement,
			Code:      gocoap.Content,
			MessageID: msg.MessageID,
			Token:     msg.Token,
			Payload:   []byte{},
		}
		res.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
	}

	if len(msg.Payload) == 0 && msg.IsConfirmable() {
		res.Code = gocoap.BadRequest
		return res
	}

	cid := mux.Var(msg, channel)
	publisher, err := ca.authorize(msg, res, cid)
	if err != nil {
		ca.logger.Log("error", fmt.Sprintf("%s", err))
		return res
	}

	n := mainflux.RawMessage{
		Channel:   cid,
		Publisher: publisher,
		Protocol:  protocol,
		Payload:   msg.Payload,
	}

	if err := ca.pubSub.Publish(n); err != nil {
		if msg.IsConfirmable() {
			res.Code = gocoap.InternalServerError
		}
		return res
	}

	if msg.IsConfirmable() {
		res.Code = gocoap.Changed
	}
	return res
}

// Observe method deals with CoAP observe messages.
func (ca *AdapterService) Observe(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
	ca.logger.Log("message", fmt.Sprintf("Got message in Observe: path=%q: %#v from %v", msg.Path(), msg, addr))
	var res *gocoap.Message

	if msg.IsConfirmable() {
		res = &gocoap.Message{
			Type:      gocoap.Acknowledgement,
			Code:      gocoap.Content,
			MessageID: msg.MessageID,
			Token:     msg.Token,
			Payload:   []byte{},
		}
		res.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
	}

	cid := mux.Var(msg, channel)
	_, err := ca.authorize(msg, res, cid)
	if err != nil {
		ca.logger.Log(err)
		return res
	}

	if value, ok := msg.Option(gocoap.Observe).(uint32); ok && value == 0 {
		subject := fmt.Sprintf("channel.%s", cid)
		if _, err := ca.pubSub.Subscribe(subject, ca.obsHandle(conn, addr, msg, 120)); err != nil {
			ca.logger.Log("error", fmt.Sprintf("Error occured during subscription to NATS %s", err))
			res.Code = gocoap.InternalServerError
			return res
		}
		res.AddOption(gocoap.Observe, 0)
	}
	return res
}
