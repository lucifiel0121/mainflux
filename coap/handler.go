package coap

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"

	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
)

// Handler is used to handle CoAP subscription.
type Handler struct {
	// Expired flag is used to mark that ticker sent a
	// CON message, but response is not received yet.
	// The flag changes its value once ACK message is
	// received from the client. If Expired is true
	// when ticker is triggered, Handler should be canceled
	// and removed from the Service map.
	expired bool
	// Messages is used to receive messages from NATS.
	Messages chan mainflux.RawMessage
	// Ticker is used to send CON message every 24 hours.
	Ticker *time.Ticker
	// Cancel channel is used to cancel observing resource.
	Cancel chan bool
	// Address represents UDP address of corresponding client.
	// Address net.UDPAddr
	msgID uint16

	expiredLock, msgIDLock sync.Mutex
}

func (h *Handler) cancel() {
	<-h.Cancel
	println("Close messages...")
	close(h.Messages)
	println("Close cancel...")
	close(h.Cancel)
	println("Stop ticker...")
	h.Ticker.Stop()
	println("Killed...")
}

func (h *Handler) sendMessage(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) error {

	if msg == nil {
		return errors.New("Nil message")
	}

	msg.MessageID = h.LoadMessageID()
	buff := new(bytes.Buffer)
	now := time.Now().UnixNano() / timestamp
	if err := binary.Write(buff, binary.BigEndian, now); err != nil {
		return err
	}

	observeVal := buff.Bytes()
	msg.SetOption(gocoap.Observe, observeVal[len(observeVal)-3:])
	return gocoap.Transmit(conn, addr, *msg)
}

func (h *Handler) handleMessage(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) {
	notifyMsg := *msg
	notifyMsg.Type = gocoap.NonConfirmable
	notifyMsg.Code = gocoap.Content
	for {
		msg, ok := <-h.Messages
		if !ok {
			return
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			// log error
		}
		notifyMsg.Payload = payload
		h.sendMessage(conn, addr, &notifyMsg)
	}
}

// LoadExpired reads Expired flag in thread-safe way.
func (h *Handler) LoadExpired() bool {
	h.expiredLock.Lock()
	defer h.expiredLock.Unlock()
	return h.expired
}

// StoreExpired stores Expired flag in thread-safe way.
func (h *Handler) StoreExpired(val bool) {
	h.expiredLock.Lock()
	defer h.expiredLock.Unlock()
	h.expired = val
}

// LoadMessageID reads MessageID and increments
// its value in thread-safe way.
func (h *Handler) LoadMessageID() uint16 {
	h.msgIDLock.Lock()
	defer h.msgIDLock.Unlock()
	h.msgID++
	return h.msgID
}

func (h *Handler) ping(svc Service, clientID string, conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) {
	pingMsg := *msg
	pingMsg.Payload = []byte{}
	pingMsg.Type = gocoap.Confirmable

	for {
		<-h.Ticker.C
		if h.LoadExpired() {
			svc.Unsubscribe(clientID)
			return
		}
		h.StoreExpired(true)
		println("pinging...")
		msg.MessageID = h.LoadMessageID()
		gocoap.Transmit(conn, addr, pingMsg)
	}
}
