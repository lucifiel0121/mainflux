package coap

import (
	"sync"
	"time"

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

	// Ticker is used to send CON message every 24 hours.
	Ticker *time.Ticker

	// Cancel channel is used to cancel observing resource.
	Cancel chan bool
}

// NewHandler instantiates a new Handler for an observer.
func NewHandler() *Handler {
	return &Handler{
		Messages: make(chan mainflux.RawMessage),
		// According to RFC (https://tools.ietf.org/html/rfc7641#page-18), CON message must be sent at least every
		// 24 hours. Since 24 hours is too long for our purposes, we use 12.
		Ticker: time.NewTicker(12 * time.Hour),
		Cancel: make(chan bool),
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
