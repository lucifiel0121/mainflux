package coap

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"

	gocoap "github.com/dustin/go-coap"
	"github.com/gogo/protobuf/proto"
	"github.com/mainflux/mainflux"
	broker "github.com/nats-io/go-nats"
)

// AuthProvider represents ManagerClient which provides auth features.
type AuthProvider interface {
	CanAccess(string, string) (string, error)
}

// ObsHandle handles observe messages and keeps connection to client in order to send notifications.
func (ca *AdapterService) obsHandle(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message, offset time.Duration) broker.MsgHandler {
	var counter uint32
	count := make([]byte, 4)
	return func(brokerMsg *broker.Msg) {
		if conn == nil || addr == nil {
			return
		}
		if brokerMsg == nil {
			ca.logger.Log("error", "Got an empty message from NATS")
			return
		}
		rawMsg := mainflux.RawMessage{}
		if err := proto.Unmarshal(brokerMsg.Data, &rawMsg); err != nil {
			ca.logger.Log("error", fmt.Sprintf("Error converting NATS message to RawMessage %s", err))
			return
		}
		counter++
		binary.LittleEndian.PutUint32(count, counter)

		msg.Type = gocoap.Confirmable
		msg.Payload = rawMsg.Payload
		msg.Code = gocoap.Content

		msg.SetOption(gocoap.Observe, count[:3])
		msg.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
		msg.SetOption(gocoap.LocationPath, msg.Path())
		ca.logger.Log("message", fmt.Sprintf("Transmitting %v", brokerMsg))
		if err := gocoap.Transmit(conn, addr, *msg); err != nil {
			ca.logger.Log("error", fmt.Sprintf("Can't notify client %s", err))
			return
		}

		conn.SetReadDeadline(time.Now().Add(time.Second * offset))
		resp, err := receive(conn)
		ca.logger.Log("message", string(resp.Payload))
		if err != nil {
			ca.logger.Log("error", fmt.Sprintf("Got error waiting to receive client answer %s", err))
			if err := brokerMsg.Sub.Unsubscribe(); err != nil {
				ca.logger.Log("error", err)
			}
		}
		if resp.Type == gocoap.Reset {
			if err := teardown(conn, brokerMsg); err != nil {
				ca.logger.Log("error", err)
			}
		} else {
			// Zero time sets deadline to no limit.
			conn.SetReadDeadline(time.Time{})
		}
	}
}

func receive(l *net.UDPConn) (gocoap.Message, error) {
	buff := make([]byte, maxPktLen)
	nr, _, err := l.ReadFromUDP(buff)
	if err != nil {
		return gocoap.Message{}, err
	}
	return gocoap.ParseMessage(buff[:nr])
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
	if len(arr) != 2 || strings.ToLower(arr[0]) != key {
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

func (ca *AdapterService) authorize(msg *gocoap.Message, res *gocoap.Message, cid string) (publisher string, err error) {
	// Device Key is passed as Uri-Query parameter which option ID is 15 (0xf).
	key, err := authKey(msg.Option(gocoap.URIQuery))
	if err != nil {
		res.Code = gocoap.BadRequest
		return
	}

	publisher, err = ca.auth.CanAccess(cid, key)
	if err != nil {
		// Note that this is not the best way to handle access error, since problem could be
		// reference to an unexisting channel, not invalid token.
		res.Code = gocoap.Unauthorized
	}
	return
}
