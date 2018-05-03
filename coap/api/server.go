package api

import (
	"fmt"
	"net"
	"strings"
	"time"

	mux "github.com/dereulenspiegel/coap-mux"
	"github.com/mainflux/mainflux/coap"
	manager "github.com/mainflux/mainflux/manager/client"

	gocoap "github.com/dustin/go-coap"
)

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
		return "", errBadOption
	}
	return arr[1], nil
}

func authorize(msg *gocoap.Message, res *gocoap.Message, cid string) (publisher string, err error) {
	// Device Key is passed as Uri-Query parameter, which option ID is 15 (0xf).
	key, err := authKey(msg.Option(gocoap.URIQuery))
	if err != nil {
		switch err {
		case errBadOption:
			res.Code = gocoap.BadOption
		case errBadRequest:
			res.Code = gocoap.BadRequest
		}
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

func serve(svc coap.Service, conn *net.UDPConn, data []byte, addr *net.UDPAddr, rh gocoap.Handler) {
	msg, err := gocoap.ParseMessage(data)
	if err != nil {
		return
	}
	res := &gocoap.Message{
		Type:      gocoap.NonConfirmable,
		Code:      gocoap.Content,
		MessageID: msg.MessageID,
		Token:     msg.Token,
		Payload:   []byte{},
	}
	switch msg.Type {
	case gocoap.Reset:
		cid := mux.Var(&msg, "id")
		res.Type = gocoap.Acknowledgement
		publisher, err := authorize(&msg, res, cid)
		if err != nil {
			res.Code = gocoap.Unauthorized
			break
		}
		id := fmt.Sprintf("%s-%x", publisher, msg.Token)
		svc.RemoveTimeout(id)
		if err := svc.Unsubscribe(id); err != nil {
			res.Code = gocoap.InternalServerError

		}
	case gocoap.Acknowledgement:
		cid := mux.Var(&msg, "id")
		res.Type = gocoap.Acknowledgement
		publisher, err := authorize(&msg, res, cid)
		if err != nil {
			res.Code = gocoap.Unauthorized
		}
		id := fmt.Sprintf("%s-%x", publisher, msg.Token)
		svc.RemoveTimeout(id)
	default:
		res = rh.ServeCOAP(conn, addr, &msg)
	}
	if res != nil && msg.IsConfirmable() {
		gocoap.Transmit(conn, addr, *res)
	}
}

// ListenAndServe binds to the given address and serve requests forever.
func ListenAndServe(svc coap.Service, mgr manager.ManagerClient, addr string, rh gocoap.Handler) error {
	auth = mgr
	uaddr, err := net.ResolveUDPAddr(network, addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP(network, uaddr)
	if err != nil {
		return err
	}

	buf := make([]byte, maxPktLen)
	for {
		nr, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if neterr, ok := err.(net.Error); ok && (neterr.Temporary() || neterr.Timeout()) {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return err
		}
		tmp := make([]byte, nr)
		copy(tmp, buf)
		go serve(svc, conn, tmp, addr, rh)
	}
}
