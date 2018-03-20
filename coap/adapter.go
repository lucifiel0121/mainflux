package coap

import (
	"errors"
	"fmt"
	"net"

	"encoding/base64"

	mux "github.com/dereulenspiegel/coap-mux"
	coap "github.com/dustin/go-coap"
	"github.com/go-kit/kit/log"
	"github.com/mainflux/mainflux"
	manager "github.com/mainflux/mainflux/manager/client"
	broker "github.com/nats-io/go-nats"
)

const (
	token    string = "token"
	prefix   string = "src.coap."
	channel  string = "id"
	protocol string = "coap"
)

var (
	errBadRequest = errors.New("bad request")
)

// Observer struct provides support to observe message types.
type Observer struct {
	conn    *net.UDPConn
	addr    *net.UDPAddr
	message *coap.Message
	sub     *broker.Subscription
}

// Subscriber represents CoAP message token-NATS subscription map. Token is represented as
// string in order to simplify access to subscription as well as to simplify unsubscribe feature.
type Subscriber map[string]*Observer

// AdapterService struct represents CoAP adapter service.
type AdapterService struct {
	observers map[string][]Observer
	logger    log.Logger
	pub       mainflux.MessagePublisher
	subs      map[string]Subscriber
	nc        *broker.Conn
	mc        *manager.ManagerClient
}

// New creates new CoAP adapter service struct.
func New(logger log.Logger, pub mainflux.MessagePublisher, nc *broker.Conn, mc *manager.ManagerClient) *AdapterService {
	ca := &AdapterService{
		logger:    logger,
		pub:       pub,
		observers: make(map[string][]Observer),
		subs:      make(map[string]Subscriber),
		nc:        nc,
		mc:        mc,
	}
	return ca
}

// Recieve method processes message and pushes it to NATS.
func (ca *AdapterService) Recieve(l *net.UDPConn, a *net.UDPAddr, m *coap.Message) *coap.Message {
	ca.logger.Log("message", fmt.Sprintf("Got message in sendMessage: path=%q: %#v from %v", m.Path(), m, a))
	var res *coap.Message

	if m.IsConfirmable() {
		res = &coap.Message{
			Type:      coap.Acknowledgement,
			Code:      coap.Content,
			MessageID: m.MessageID,
			Token:     m.Token,
			Payload:   []byte(""),
		}
		res.SetOption(coap.ContentFormat, coap.AppJSON)
	}

	if len(m.Payload) == 0 && m.IsConfirmable() {
		res.Code = coap.BadRequest
		return res
	}

	cid := mux.Var(m, channel)

	// Uri-Query option ID is 15 (0xf).
	token, err := authToken(m.Option(0xf))
	if err != nil {
		res.Code = coap.BadRequest
		return res
	}

	publisher, err := ca.mc.CanAccess(cid, token)
	if err != nil {
		ca.logger.Log(err)
		res.Code = coap.Unauthorized
		return res
	}

	n := mainflux.RawMessage{
		Channel:   cid,
		Publisher: publisher,
		Protocol:  protocol,
		Payload:   m.Payload,
	}

	// Publish to channel. Since other adapters are subscribed to
	// channel with src.coap.* wildcard, everybody will get message.
	if err := ca.pub.Publish(n); err != nil {
		if m.IsConfirmable() {
			res.Code = coap.InternalServerError
		}
		return res
	}

	if m.IsConfirmable() {
		res.Code = coap.Changed
	}
	return res
}

func (ca *AdapterService) notify(nm *broker.Msg) {
	m, err := convertMsg(nm)
	if err != nil {
		ca.logger.Log("error", fmt.Sprintf("Can't convert NATS message to RawMesage: %s", err))
		return
	}
	ca.logger.Log(m.Publisher, m.Protocol, m.Channel, m.Payload)
	ca.transmit(m.Channel, m.Payload, m.Publisher)
}

// Observe method deals with CoAP observe messages.
func (ca *AdapterService) Observe(l *net.UDPConn, a *net.UDPAddr, m *coap.Message) *coap.Message {
	ca.logger.Log("message", fmt.Sprintf("Got message in ObserveMessage: path=%q: %#v from %v", m.Path(), m, a))
	var res *coap.Message
	if m.IsConfirmable() {
		res = &coap.Message{
			Type:      coap.Acknowledgement,
			Code:      coap.Content,
			MessageID: m.MessageID,
			Token:     m.Token,
			Payload:   []byte(""),
		}
		res.SetOption(coap.ContentFormat, coap.AppJSON)
	}

	// Uri-Query option ID is 15 (0xf).
	token, err := authToken(m.Option(0xf))
	if err != nil {
		res.Code = coap.BadRequest
		return res
	}

	cid := mux.Var(m, channel)
	// Note that this is not the best way to handle access, since problem could be
	// reference to an unexisting channel, not invalid token.
	if _, err = ca.mc.CanAccess(cid, token); err != nil {
		println(err.Error())
		res.Code = coap.Unauthorized
		return res
	}

	if value, ok := m.Option(coap.Observe).(uint32); ok && value == 0 {
		o := &Observer{
			conn:    l,
			addr:    a,
			message: m,
		}
		ca.subscribe(o, cid)
	} else {
		ca.unsubscribe(m, cid)
	}

	return res
}

func (ca *AdapterService) subscribe(o *Observer, cid string) error {
	res, err := ca.nc.Subscribe(cid, ca.notify)
	s := make(map[string]*Observer)
	if err != nil {
		return err
	}
	o.message.Code = coap.Valid
	o.sub = res
	token := base64.StdEncoding.EncodeToString(o.message.Token)
	s[token] = o
	ca.subs[cid] = s
	ca.logger.Log("message", "Subscribed to: "+ca.subs[cid][token].sub.Subject)
	return nil
}

func (ca *AdapterService) unsubscribe(m *coap.Message, cid string) error {
	sub, ok := ca.subs[cid]
	if !ok {
		return nil
	}
	token := base64.StdEncoding.EncodeToString(m.Token)
	s, ok := sub[token]
	if !ok {
		return nil
	}
	ca.logger.Log("message", "Unsubscribed from: "+ca.subs[cid][token].sub.Subject)
	m.Code = coap.Valid
	err := s.sub.Unsubscribe()
	delete(ca.subs[cid], token)
	return err
}

// transmit method sends responses to subscribers.
func (ca *AdapterService) transmit(cid string, payload []byte, publisher string) {
	for _, v := range ca.subs[cid] {
		msg := *(v.message)
		msg.Payload = payload
		msg.SetOption(coap.ContentFormat, coap.AppJSON)
		msg.SetOption(coap.LocationPath, msg.Path())
		msg.Code = coap.Content
		ca.logger.Log("message", fmt.Sprintf("Transmitting %v", msg))
		err := coap.Transmit(v.conn, v.addr, msg)
		if err != nil {
			ca.logger.Log("error", fmt.Sprintf("Error on transmitter, stopping: %s", err))
			return
		}
	}
}
