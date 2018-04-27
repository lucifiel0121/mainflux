package coap

import (
	"errors"
	"fmt"
	"net"
	"time"

	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
)

var (
	// ErrFailedMessagePublish indicates that message publishing failed.
	ErrFailedMessagePublish = errors.New("failed to publish message")

	// ErrFailedSubscription indicates that client couldn't subscribe to specified channel.
	ErrFailedSubscription = errors.New("failed to subscribe to a channel")

	// ErrFailedConnection indicates that service couldn't connect to message broker.
	ErrFailedConnection = errors.New("failed to connect to message broker")
)

// Subscription interface represents subscription to messaging system.
type Subscription interface {
	Unsubscribe() error
}

// Observer struct represents observer of CoAP messages.
type Observer struct {
	sub   Subscription
	msgCh chan mainflux.RawMessage
}

// Service specifies coap service API.
type Service interface {
	mainflux.MessagePublisher
	// Subscribes to channel with specified id.
	Subscribe(string, Channel) (Subscription, error)
}

// Channel is used for receiving and sending messages.
type Channel struct {
	Messages chan mainflux.RawMessage
	Closed   chan bool
}

// Close channel and stop message transfer.
func (channel Channel) Close() {
	close(channel.Messages)
	close(channel.Closed)
}

// Handler is a type that handles CoAP messages.

func handlePacket(l *net.UDPConn, data []byte, u *net.UDPAddr,
	rh gocoap.Handler) {

	msg, err := gocoap.ParseMessage(data)
	if err != nil {
		return
	}

	rv := rh.ServeCOAP(l, u, &msg)
	if rv != nil {
		Transmit(l, u, *rv)
	}
}

// Transmit a message.
func Transmit(l *net.UDPConn, a *net.UDPAddr, m gocoap.Message) error {
	d, err := m.MarshalBinary()
	if err != nil {
		return err
	}

	if a == nil {
		_, err = l.Write(d)
	} else {
		_, err = l.WriteTo(d, a)
	}
	return err
}

// Receive a message.
func Receive(l *net.UDPConn, buf []byte) (gocoap.Message, error) {
	l.SetReadDeadline(time.Now().Add(gocoap.ResponseTimeout))

	nr, _, err := l.ReadFromUDP(buf)
	if err != nil {
		return gocoap.Message{}, err
	}
	return gocoap.ParseMessage(buf[:nr])
}

// ListenAndServe binds to the given address and serve requests forever.
func ListenAndServe(n, addr string, rh gocoap.Handler) error {
	uaddr, err := net.ResolveUDPAddr(n, addr)
	if err != nil {
		return err
	}

	l, err := net.ListenUDP(n, uaddr)
	if err != nil {
		return err
	}

	return serve(l, rh)
}

func serve(listener *net.UDPConn, rh gocoap.Handler) error {
	buf := make([]byte, maxPktLen)
	for {
		nr, addr, err := listener.ReadFromUDP(buf)
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
		go handlePacket(listener, tmp, addr, rh)
	}
}
