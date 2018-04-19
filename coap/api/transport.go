package api

import (
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/mainflux/mainflux/coap"
	manager "github.com/mainflux/mainflux/manager/client"
	broker "github.com/nats-io/go-nats"

	mux "github.com/dereulenspiegel/coap-mux"
	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
)

var (
	auth          manager.ManagerClient
	errBadRequest = errors.New("bad request")
	errBadOption  = errors.New("bad option")
)

const (
	protocol  = "coap"
	maxPktLen = 1500
)

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
func MakeHandler(svc coap.Service, mgr manager.ManagerClient) gocoap.Handler {
	auth = mgr
	r := mux.NewRouter()
	r.Handle("/channels/{id}/messages", gocoap.FuncHandler(receive(svc))).Methods(gocoap.POST)
	r.Handle("/channels/{id}/messages", gocoap.FuncHandler(observe(svc))).Methods(gocoap.GET)
	r.NotFoundHandler = gocoap.FuncHandler(NotFoundHandler)
	return r
}

func receive(svc coap.Service) func(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
	return func(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
		var res *gocoap.Message
		println("received message: ", string(msg.Payload))
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

		cid := mux.Var(msg, "id")
		publisher, err := authorize(msg, res, cid)
		if err != nil {
			print(err.Error())
			return res
		}

		n := mainflux.RawMessage{
			Channel:   cid,
			Publisher: publisher,
			Protocol:  protocol,
			Payload:   msg.Payload,
		}

		if err := svc.Publish(n); err != nil {
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
}

func observe(svc coap.Service) func(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
	return func(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
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

		cid := mux.Var(msg, "id")
		_, err := authorize(msg, res, cid)
		if err != nil {
			print(err.Error())
			return res
		}
		if value, ok := msg.Option(gocoap.Observe).(uint32); ok && value == 0 {
			channel := coap.Channel{make(chan mainflux.RawMessage), make(chan bool)}
			if err := svc.Subscribe(cid, channel); err != nil {
				println(err.Error())
				res.Code = gocoap.InternalServerError
				return res
			}
			handleSubscribe(conn, addr, msg, 6000, channel)
			res.AddOption(gocoap.Observe, 0)
		}
		return res
	}
}

func handleSubscribe(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message, offset time.Duration, channel coap.Channel) {
	go func() {
		var counter uint32
		count := make([]byte, 4)
		for {
			rawMsg := <-channel.Messages
			println(string(rawMsg.Payload))
			counter++
			binary.LittleEndian.PutUint32(count, counter)

			msg.Type = gocoap.Confirmable
			msg.Code = gocoap.Content
			msg.Payload = rawMsg.Payload

			msg.SetOption(gocoap.Observe, count[:3])
			msg.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
			msg.SetOption(gocoap.LocationPath, msg.Path())
			if err := gocoap.Transmit(conn, addr, *msg); err != nil {
				println("Trnsmitting error")
				return
			}
			conn.SetReadDeadline(time.Now().Add(time.Millisecond * offset))
			resp, err := response(conn)
			if err != nil {
				print("got errror waiting for clinet to response...")
				conn.Close()
				channel.Closed <- true
				return
				// 	if err := brokerMsg.Sub.Unsubscribe(); err != nil {
			}
			println(resp.Code)
			// 	return
			// }
			// if resp.Type == gocoap.Reset {
			// 	if err := teardown(conn, brokerMsg); err != nil {
			// 	}
			// } else {
			// Zero time sets deadline to no limit.
			conn.SetReadDeadline(time.Time{})
		}
	}()
}

func response(conn *net.UDPConn) (gocoap.Message, error) {
	buff := make([]byte, maxPktLen)
	nr, _, err := conn.ReadFromUDP(buff)
	if err != nil {
		conn.Close()
		return gocoap.Message{}, err
	}
	return gocoap.ParseMessage(buff[:nr])
}

func teardown(conn *net.UDPConn, msg *broker.Msg) error {
	if err := conn.Close(); err != nil {
		return err
	}
	return msg.Sub.Unsubscribe()
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
	if len(arr) != 2 || strings.ToLower(arr[0]) != "key" {
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
			res.Code = gocoap.ServiceUnavailable
		default:
			res.Code = gocoap.Unauthorized
		}
	}
	return
}
