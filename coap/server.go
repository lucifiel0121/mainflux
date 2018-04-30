package coap

import (
	"fmt"
	"net"
	"strings"
	"time"

	mux "github.com/dereulenspiegel/coap-mux"
	manager "github.com/mainflux/mainflux/manager/client"

	gocoap "github.com/dustin/go-coap"
)

var auth manager.ManagerClient

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

// Authorize method is used to authorize request.
func Authorize(msg *gocoap.Message, res *gocoap.Message, cid string) (publisher string, err error) {
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

func serve(svc Service, conn *net.UDPConn, data []byte, addr *net.UDPAddr, rh gocoap.Handler) {
	msg, err := gocoap.ParseMessage(data)
	if err != nil {
		return
	}
	var respMsg *gocoap.Message
	switch msg.Type {
	case gocoap.Reset:
		cid := mux.Var(&msg, "id")
		publisher, err := Authorize(&msg, &msg, cid)
		if err != nil {
			return
		}
		id := fmt.Sprintf("%s-%x", publisher, msg.Token)
		svc.Unsubscribe(id)
		respMsg = &msg
		respMsg.Type = gocoap.Acknowledgement
	default:
		respMsg = rh.ServeCOAP(conn, addr, &msg)
	}
	if respMsg != nil {
		gocoap.Transmit(conn, addr, *respMsg)
	}
}

// ListenAndServe binds to the given address and serve requests forever.
func ListenAndServe(svc Service, mgr manager.ManagerClient, addr string, rh gocoap.Handler) error {
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
		fmt.Printf("received: %d, %s:%d\n", nr, addr.IP, addr.Port)
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
