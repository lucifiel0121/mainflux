package coap

import (
	"fmt"
	"net"
	"time"

	gocoap "github.com/dustin/go-coap"
)

func serve(svc Service, conn *net.UDPConn, data []byte, addr *net.UDPAddr, rh gocoap.Handler) {
	msg, err := gocoap.ParseMessage(data)
	if err != nil {
		return
	}
	var respMsg *gocoap.Message
	switch msg.Type {
	case gocoap.Reset:
		svc.Unsubscribe(addr, &msg)
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
func ListenAndServe(svc Service, addr string, rh gocoap.Handler) error {
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
