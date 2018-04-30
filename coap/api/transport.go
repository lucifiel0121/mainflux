package api

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/mainflux/mainflux/coap"
	manager "github.com/mainflux/mainflux/manager/client"

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
func MakeHandler(svc coap.Service) gocoap.Handler {
	r := mux.NewRouter()
	r.Handle("/channels/{id}/messages", gocoap.FuncHandler(receive(svc))).Methods(gocoap.POST)
	r.Handle("/channels/{id}/messages", gocoap.FuncHandler(observe(svc))).Methods(gocoap.GET)
	r.NotFoundHandler = gocoap.FuncHandler(NotFoundHandler)
	return r
}

func receive(svc coap.Service) func(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
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

		if len(msg.Payload) == 0 && msg.IsConfirmable() {
			res.Code = gocoap.BadRequest
			return res
		}

		cid := mux.Var(msg, "id")
		publisher, err := coap.Authorize(msg, res, cid)
		if err != nil {
			res.Code = gocoap.Unauthorized
			return res
		}

		rawMsg := mainflux.RawMessage{
			Channel:   cid,
			Publisher: publisher,
			Protocol:  protocol,
			Payload:   msg.Payload,
		}

		if err := svc.Publish(rawMsg); err != nil {
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
		publisher, err := coap.Authorize(msg, res, cid)

		if err != nil {
			res.Code = gocoap.Unauthorized
			return res
		}

		if value, ok := msg.Option(gocoap.Observe).(uint32); ok && value == 1 {
			id := fmt.Sprintf("%s-%x", publisher, msg.Token)
			err := svc.Unsubscribe(id)
			if err != nil {
				res.Code = gocoap.InternalServerError
			}
		}

		if value, ok := msg.Option(gocoap.Observe).(uint32); ok && value == 0 {
			ch := make(chan mainflux.RawMessage)
			id := fmt.Sprintf("%s-%x", publisher, msg.Token)
			if err := svc.Subscribe(cid, id, ch); err != nil {
				res.Code = gocoap.InternalServerError
				return res
			}
			go handleSub(svc, id, conn, addr, msg, ch)
			res.AddOption(gocoap.Observe, 0)
		}
		return res
	}
}

func notify(svc coap.Service, id string, conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message, counter *uint16) error {
	err := sendMessage(conn, addr, msg, counter)
	if err != nil {
		return svc.Unsubscribe(id)
	}
	return nil
}

func sendMessage(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message, counter *uint16) error {
	var err error
	*counter++
	now := time.Now().UnixNano() / int64(time.Millisecond)
	buff := new(bytes.Buffer)
	err = binary.Write(buff, binary.LittleEndian, now)
	if err != nil {
		return err
	}
	msg.MessageID = *counter
	msg.SetOption(gocoap.Observe, buff.Bytes()[:3])
	for i := 0; i < 3; i++ {
		err = gocoap.Transmit(conn, addr, *msg)
		if err != nil {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		return nil
	}
	return err
}

func handleSub(svc coap.Service, id string, conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message, ch chan mainflux.RawMessage) {
	counter := msg.MessageID
	ticker := time.NewTicker(24 * time.Hour)
	res := &gocoap.Message{
		Type:      gocoap.NonConfirmable,
		Code:      gocoap.Content,
		MessageID: msg.MessageID,
		Token:     msg.Token,
		Payload:   []byte{},
	}
	res.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
	res.SetOption(gocoap.LocationPath, msg.Path())

	func() {
		for {
			select {
			case <-ticker.C:
				res.Type = gocoap.Confirmable
				if err := notify(svc, id, conn, addr, res, &counter); err != nil {
					return
				}
			case rawMsg, ok := <-ch:
				if !ok {
					return
				}
				res.Type = gocoap.NonConfirmable
				res.Payload = rawMsg.Payload
				if err := notify(svc, id, conn, addr, res, &counter); err != nil {
					return
				}
			}
		}
	}()
	ticker.Stop()
	println("worker finished...")
}
