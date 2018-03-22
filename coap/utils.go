package coap

import (
	"fmt"
	"net"
	"strings"
	"time"

	gocoap "github.com/dustin/go-coap"
	"github.com/gogo/protobuf/proto"
	"github.com/mainflux/mainflux"
	broker "github.com/nats-io/go-nats"
)

// obsHandle handles observe messages and keeps connection to client in order to send notifications.
func (ca *AdapterService) obsHandle(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message, offset time.Duration, pub string) broker.MsgHandler {
	return func(msg *broker.Msg) {
		rawMsg := mainflux.RawMessage{}
		proto.Unmarshal(msg.Data, &rawMsg)
		if rawMsg.Publisher == pub {
			// Don't notify yourself.
			return
		}
		m.Type = gocoap.Confirmable
		m.Payload = rawMsg.Payload
		m.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
		m.SetOption(gocoap.LocationPath, m.Path())
		m.Code = gocoap.Content
		ca.logger.Log("message", fmt.Sprintf("Transmitting %v", msg))
		if err := gocoap.Transmit(l, a, *m); err != nil {
			ca.logger.Log("error", err)
		}
		buff := []byte{}
		l.SetReadDeadline(time.Now().Add(time.Second * offset))

		resp, err := receive(l, buff)
		if err != nil {
			ca.logger.Log("error", err)
			if err := msg.Sub.Unsubscribe(); err != nil {
				ca.logger.Log("error", err)
			}
		}
		if resp.Type == gocoap.Reset {
			if err := teardown(l, msg); err != nil {
				ca.logger.Log("error", err)
			}
		}
	}
}

func receive(l *net.UDPConn, buf []byte) (gocoap.Message, error) {
	nr, _, err := l.ReadFromUDP(buf)
	if err != nil {
		return gocoap.Message{}, err
	}
	return gocoap.ParseMessage(buf[:nr])
}

func authKey(opt interface{}) (string, error) {
	if opt == nil {
		return "", errBadRequest
	}
	val, ok := opt.(string)
	if !ok {
		return "", errBadRequest
	}
	arr := strings.Split(val, "=")
	if len(arr) != 2 || strings.ToLower(arr[0]) != "key" {
		return "", errBadRequest
	}
	return arr[1], nil
}

func teardown(conn *net.UDPConn, msg *broker.Msg) error {
	if err := conn.Close(); err != nil {
		return err
	}
	return msg.Sub.Unsubscribe()
}

func authorize(msg *gocoap.Message, res *gocoap.Message, cid string, access func(string, string) (string, error)) (publisher string, err error) {
	// Device Key is passed as Uri-Query parameter which option ID is 15 (0xf).
	key, err := authKey(msg.Option(0xf))
	if err != nil {
		res.Code = gocoap.BadRequest
		return
	}

	publisher, err = access(cid, key)
	if err != nil {
		// Note that this is not the best way to handle access, since problem could be
		// reference to an unexisting channel, not invalid token.
		res.Code = gocoap.Unauthorized
	}
	return
}
