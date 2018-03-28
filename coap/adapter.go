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
	Publish(msg mainflux.RawMessage) error
	Subscribe(subject string, cb broker.MsgHandler) (*broker.Subscription, error)
}

// MsgHandler handles messages CoAP server recieved.
type MsgHandler func(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message) *gocoap.Message

// Service reprents actual CoAP adapter service.
type Service interface {
	Observe(auth AuthProvider) MsgHandler
	Recieve(auth AuthProvider) MsgHandler
}

const (
	key       string = "key"
	channel   string = "id"
	protocol  string = "coap"
	maxPktLen int    = 1500
)

var (
	errBadRequest = errors.New("bad request")
	logger        log.Logger
)

// AdapterService struct represents CoAP adapter service implementation.
type AdapterService struct {
	logger log.Logger
	pubSub PubSub
}

// New creates new CoAP adapter service struct.
func New(lgr log.Logger, pubSub PubSub) Service {
	logger = lgr
	ca := &AdapterService{
		logger: logger,
		pubSub: pubSub,
	}
	return ca
}

// Recieve method processes message and pushes it to NATS.
func (ca *AdapterService) Recieve(auth AuthProvider) MsgHandler {
	return func(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message) *gocoap.Message {
		ca.logger.Log("message", fmt.Sprintf("Got message in Recieve: path=%q: %#v from %v", m.Path(), m, a))
		var res *gocoap.Message

		if m.IsConfirmable() {
			res = &gocoap.Message{
				Type:      gocoap.Acknowledgement,
				Code:      gocoap.Content,
				MessageID: m.MessageID,
				Token:     m.Token,
				Payload:   []byte{},
			}
			res.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
		}

		if len(m.Payload) == 0 && m.IsConfirmable() {
			res.Code = gocoap.BadRequest
			return res
		}

		cid := mux.Var(m, channel)
		publisher, err := authorize(m, res, cid, auth)
		if err != nil {
			ca.logger.Log("error", fmt.Sprintf("%s", err))
			return res
		}

		n := mainflux.RawMessage{
			Channel:   cid,
			Publisher: publisher,
			Protocol:  protocol,
			Payload:   m.Payload,
		}

		if err := ca.pubSub.Publish(n); err != nil {
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
}

// Observe method deals with CoAP observe messages.
func (ca *AdapterService) Observe(auth AuthProvider) MsgHandler {
	return func(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message) *gocoap.Message {
		ca.logger.Log("message", fmt.Sprintf("Got message in Observe: path=%q: %#v from %v", m.Path(), m, a))
		var res *gocoap.Message

		if m.IsConfirmable() {
			res = &gocoap.Message{
				Type:      gocoap.Acknowledgement,
				Code:      gocoap.Content,
				MessageID: m.MessageID,
				Token:     m.Token,
				Payload:   []byte{},
			}
			res.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
		}

		cid := mux.Var(m, channel)
		_, err := authorize(m, res, cid, auth)
		if err != nil {
			ca.logger.Log(err)
			return res
		}

		if value, ok := m.Option(gocoap.Observe).(uint32); ok && value == 0 {
			subject := fmt.Sprintf("channel.%s", cid)
			if _, err := ca.pubSub.Subscribe(subject, obsHandle(l, a, m, 120)); err != nil {
				ca.logger.Log("error", fmt.Sprintf("Error occured during subscription to NATS %s", err))
				res.Code = gocoap.InternalServerError
				return res
			}
			res.AddOption(gocoap.Observe, 0)
		} else {
			// TODO Handle explicit unsubscription.
		}
		return res
	}
}
