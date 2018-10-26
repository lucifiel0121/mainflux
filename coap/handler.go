package coap

import (
	"sync"

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

	// Message ID for notification messages.
	msgID uint16

	expiredLock, msgIDLock sync.Mutex

	// Messages is used to receive messages from NATS.
	Messages chan mainflux.RawMessage

	// Cancel channel is used to cancel observing resource.
	// Cancel channel should not be used to send or receive any
	// data, it's purpose is to be closed once handler canceled.
	Cancel chan bool
}

// NewHandler instantiates a new Handler for an observer.
func NewHandler() *Handler {
	return &Handler{
		Messages: make(chan mainflux.RawMessage),
		Cancel:   make(chan bool),
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
