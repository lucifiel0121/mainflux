package api

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	manager "github.com/mainflux/mainflux/manager/client"
	broker "github.com/nats-io/go-nats"

	mux "github.com/dereulenspiegel/coap-mux"
	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
)

var auth manager.ManagerClient

// NotFoundHandler handles erroneusly formed requests.
func NotFoundHandler(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message) *gocoap.Message {
	if m.IsConfirmable() {
		return &gocoap.Message{
			Type: gocoap.Acknowledgement,
			Code: gocoap.NotFound,
		}
	}
	return nil
}

// MakeHandler function return new CoAP server with GET, POST and NOT_FOUND handlers.
func MakeHandler() gocoap.Handler {
	r := mux.NewRouter()
	r.Handle("/channels/{id}/messages", gocoap.FuncHandler(receive)).Methods(gocoap.POST)
	r.Handle("/channels/{id}/messages", gocoap.FuncHandler(observe)).Methods(gocoap.GET)
	r.NotFoundHandler = gocoap.FuncHandler(NotFoundHandler)
	return r
}

func receive(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
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
	publisher, err := authorize(msg, res, cid)
	if err != nil {
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

func observe(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
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
	_, err := authorize(msg, res, cid)
	if err != nil {
		ca.logger.Log(err)
		return res
	}

	if value, ok := msg.Option(gocoap.Observe).(uint32); ok && value == 0 {
		subject := fmt.Sprintf("channel.%s", cid)
		if _, err := ca.pubSub.Subscribe(subject, obsHandle(conn, addr, msg, 60000)); err != nil {
			ca.logger.Log("error", fmt.Sprintf("Error occured during subscription to NATS %s", err))
			res.Code = gocoap.InternalServerError
			return res
		}
		res.AddOption(gocoap.Observe, 0)
	}
	return res
}

func obsHandle(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message, offset time.Duration) broker.MsgHandler {
	var counter uint32
	count := make([]byte, 4)
	return func(brokerMsg *broker.Msg) {
		if conn == nil || addr == nil {
			return
		}
		if brokerMsg == nil {
			return
		}
		rawMsg := mainflux.RawMessage{}
		if err := proto.Unmarshal(brokerMsg.Data, &rawMsg); err != nil {
			return
		}
		counter++
		binary.LittleEndian.PutUint32(count, counter)

		msg.Type = gocoap.Confirmable
		msg.Code = gocoap.Content
		msg.Payload = rawMsg.Payload

		msg.SetOption(gocoap.Observe, count[:3])
		msg.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
		msg.SetOption(gocoap.LocationPath, msg.Path())
		if err := gocoap.Transmit(conn, addr, *msg); err != nil {
			return
		}

		conn.SetReadDeadline(time.Now().Add(time.Millisecond * offset))
		resp, err := receive(conn)
		ca.logger.Log("message", string(resp.Payload))
		if err != nil {
			if err := brokerMsg.Sub.Unsubscribe(); err != nil {
			}
			return
		}
		if resp.Type == gocoap.Reset {
			if err := teardown(conn, brokerMsg); err != nil {
			}
		} else {
			// Zero time sets deadline to no limit.
			conn.SetReadDeadline(time.Time{})
		}
	}
}

func authKey(opt interface{}) (string, gocoap.COAPCode, error) {
	if opt == nil {
		return "", gocoap.BadRequest, errBadRequest
	}
	val, ok := opt.(string)
	if !ok {
		return "", gocoap.BadRequest, errBadRequest
	}
	arr := strings.Split(val, "=")
	if len(arr) != 2 || strings.ToLower(arr[0]) != key {
		return "", gocoap.BadOption, errBadOption
	}
	return arr[1], gocoap.Valid, nil
}

func authorize(msg *gocoap.Message, res *gocoap.Message, cid string) (publisher string, err error) {
	// Device Key is passed as Uri-Query parameter, which option ID is 15 (0xf).
	key, code, err := authKey(msg.Option(gocoap.URIQuery))
	if err != nil {
		res.Code = code
		return
	}

	publisher, err = auth.CanAccess(cid, key)
	if err != nil {
		switch err {
		case manager.ErrServiceUnreachable:
			res.code = gocoap.ServiceUnavailable
		default:
			res.Code = gocoap.Unauthorized
		}
	}
	return
}
