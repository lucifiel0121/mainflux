package coap

import (
	"encoding/json"
	"fmt"
	"net"

	"encoding/base64"

	mux "github.com/dereulenspiegel/coap-mux"
	coap "github.com/dustin/go-coap"
	"github.com/go-kit/kit/log"
	"github.com/mainflux/mainflux"
	broker "github.com/nats-io/go-nats"
)

const (
	protocol string = "coap"
	channel  string = "channel_id"
	chPrefix string = "src.coap."
)

// Observer struct provides support to observe message types.
type Observer struct {
	conn    *net.UDPConn
	addr    *net.UDPAddr
	message *coap.Message
	sub     *broker.Subscription
}

// Subscriber represents token-NATS subscription map. Token is represented as string in
// order to simplify access to subscription as well as to simplify unsubscribe feature.
type Subscriber map[string]*Observer

// AdapterService struct represents CoAP adapter service.
type AdapterService struct {
	obsMap map[string][]Observer
	logger log.Logger
	pub    mainflux.MessagePublisher
	subs   map[string]Subscriber
	nc     *broker.Conn
}

// New creates new CoAP adapter service struct.
func New(logger log.Logger, pub mainflux.MessagePublisher, nc *broker.Conn) *AdapterService {
	ca := &AdapterService{
		logger: logger,
		pub:    pub,
		obsMap: make(map[string][]Observer),
		subs:   make(map[string]Subscriber),
		nc:     nc,
	}
	return ca
}

// SendMessage method sends processes message and pushes it to NATS.
func (ca *AdapterService) SendMessage(l *net.UDPConn, a *net.UDPAddr, m *coap.Message) *coap.Message {
	ca.logger.Log("info", fmt.Sprintf("Got message in sendMessage: path=%q: %#v from %v", m.Path(), m, a))
	var res *coap.Message

	if len(m.Payload) == 0 && m.IsConfirmable() {
		res.Code = coap.BadRequest
		return res
	}
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

	// Channel ID
	cid := chPrefix + mux.Var(m, channel)

	n := mainflux.RawMessage{
		Channel:   cid,
		Publisher: "", // TODO authorize
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
	// TODO Authentication has to be added in order to prevent self-notifying.
	m, err := ca.convertMsg(nm)
	if err != nil {
		return
	}
	ca.logger.Log(m.Publisher, m.Protocol, m.Channel, m.Payload)
	ca.Transmit(m.Channel, m.Payload, m.Publisher)
}

// ObserveMessage method deals with CoAP observe messages.
func (ca *AdapterService) ObserveMessage(l *net.UDPConn, a *net.UDPAddr, m *coap.Message) *coap.Message {
	ca.logger.Log("info", fmt.Sprintf("Got message in ObserveMessage: path=%q: %#v from %v", m.Path(), m, a))
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

	o := &Observer{
		conn:    l,
		addr:    a,
		message: m,
	}

	if value, ok := m.Option(coap.Observe).(uint32); ok && value == 0 {
		ca.registerSub(o)
	} else {
		ca.deregisterSub(m)
	}

	if m.IsConfirmable() {
		res.Code = coap.Valid
	}
	return res
}

func (ca *AdapterService) registerSub(o *Observer) error {
	cid := chPrefix + mux.Var(o.message, channel)
	res, err := ca.nc.Subscribe(cid, ca.notify)
	s := make(map[string]*Observer)
	if err != nil {
		return err
	}
	o.sub = res
	token := base64.StdEncoding.EncodeToString(o.message.Token)
	s[token] = o
	ca.subs[cid] = s
	ca.logger.Log("info", "Subscribed to: "+ca.subs[cid][token].sub.Subject)
	return nil
}

func (ca *AdapterService) deregisterSub(m *coap.Message) error {
	cid := chPrefix + mux.Var(m, channel)
	sub, ok := ca.subs[cid]
	if !ok {
		return nil
	}
	token := base64.StdEncoding.EncodeToString(m.Token)
	s, ok := sub[token]
	if !ok {
		return nil
	}
	ca.logger.Log("info", "Unsubscribed from: "+ca.subs[cid][token].sub.Subject)
	delete(ca.subs[cid], token)
	return s.sub.Unsubscribe()
}

// Transmit method sends responses to subscribers.
func (ca *AdapterService) Transmit(cid string, payload []byte, publisher string) {
	for _, v := range ca.subs[cid] {
		// TODO Add publisher check.
		msg := *(v.message)
		msg.Payload = payload
		msg.SetOption(coap.ContentFormat, coap.AppJSON)
		msg.SetOption(coap.LocationPath, msg.Path())

		ca.logger.Log("info", fmt.Sprintf("Transmitting %v", msg))
		err := coap.Transmit(v.conn, v.addr, msg)
		if err != nil {
			ca.logger.Log("error", fmt.Sprintf("Error on transmitter, stopping: %s", err))
			return
		}
	}
}

func (ca *AdapterService) convertMsg(nm *broker.Msg) (*mainflux.RawMessage, error) {
	m := mainflux.RawMessage{}
	if len(nm.Data) > 0 {
		if err := json.Unmarshal(nm.Data, &m); err != nil {
			ca.logger.Log("error", fmt.Sprintf("Can't convert NATS message to RawMesage: %s", err))
			return nil, err
		}
	}
	return &m, nil
}

// BridgeHandler functions is a handler for messages recieved via NATS.
func (ca *AdapterService) BridgeHandler(nm *broker.Msg) {
	ca.logger.Log("info", "Received a message: %s\n", string(nm.Data))
	m, err := ca.convertMsg(nm)
	if err != nil {
		return
	}
	ca.Transmit(m.Channel, m.Payload, m.Publisher)
}
