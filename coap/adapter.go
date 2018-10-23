//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

// Package coap contains the domain concept definitions needed to support
// Mainflux coap adapter service functionality. All constant values are taken
// from RFC, and could be adjusted based on specific use case.
package coap

import (
	"errors"
	"math"
	"net"
	"sync"
	"time"

	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap/nats"
	broker "github.com/nats-io/go-nats"
)

const (
	responseBackoffMultiplier = 1.5

	// AckTimeout is the amount of time to wait for a response.
	AckTimeout = int(2 * time.Second)

	// MaxRetransmit is the maximum number of times a message will be retransmitted.
	MaxRetransmit = 4

	chanID    = "id"
	keyHeader = "key"

	// Approximately number of supported requests per second
	timestamp = int64(time.Millisecond) * 31
)

var (
	errBadOption = errors.New("bad option")
	// ErrFailedMessagePublish indicates that message publishing failed.
	ErrFailedMessagePublish = errors.New("failed to publish message")

	// ErrFailedSubscription indicates that client couldn't subscribe to specified channel.
	ErrFailedSubscription = errors.New("failed to subscribe to a channel")

	// ErrFailedConnection indicates that service couldn't connect to message broker.
	ErrFailedConnection = errors.New("failed to connect to message broker")

	maxTimeout = int(float64(AckTimeout) * ((math.Pow(2, float64(MaxRetransmit))) - 1) * responseBackoffMultiplier)
)

// Service specifies coap service API.
type Service interface {
	mainflux.MessagePublisher

	// Subscribes to channel with specified id and adds subscription to
	// service map of subscriptions under given ID.
	Subscribe(uint64, string, *net.UDPAddr, *gocoap.Message) error

	// Unsubscribe method is used to stop observing resource.
	Unsubscribe(string)

	// Handle method handles ACK messages received from pinging
	// client.
	Handle(clientID string)
}

var _ Service = (*adapterService)(nil)

type adapterService struct {
	pubsub nats.Service
	subs   map[string]*Handler
	mu     sync.Mutex
	conn   *net.UDPConn
}

// New instantiates the CoAP adapter implementation.
func New(pubsub nats.Service, conn *net.UDPConn) Service {
	return &adapterService{
		pubsub: pubsub,
		subs:   make(map[string]*Handler),
		mu:     sync.Mutex{},
		conn:   conn,
	}
}

func (svc *adapterService) get(clientID string) (*Handler, bool) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	obs, ok := svc.subs[clientID]
	return obs, ok
}

func (svc *adapterService) put(clientID string, handler *Handler) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.subs[clientID] = handler
}

func (svc *adapterService) remove(clientID string) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	h, ok := svc.subs[clientID]
	if ok {
		delete(svc.subs, clientID)
		h.Cancel <- false
	}
}

func (svc *adapterService) Publish(msg mainflux.RawMessage) error {
	if err := svc.pubsub.Publish(msg); err != nil {
		switch err {
		case broker.ErrConnectionClosed, broker.ErrInvalidConnection:
			return ErrFailedConnection
		default:
			return ErrFailedMessagePublish
		}
	}
	return nil
}

func (svc *adapterService) Subscribe(chanID uint64, clientID string, clientAddr *net.UDPAddr, msg *gocoap.Message) error {
	// Remove entry if already exists.
	svc.remove(clientID)
	handler := Handler{
		Messages: make(chan mainflux.RawMessage),
		// According to RFC (https://tools.ietf.org/html/rfc7641#page-18), CON message must be sent at least every
		// 24 hours. Since 24 hours is too long for our purposes, we use 12.
		Ticker: time.NewTicker(12 * time.Second),
		Cancel: make(chan bool),
	}

	go handler.cancel()
	go handler.ping(svc, clientID, svc.conn, clientAddr, msg)
	go handler.handleMessage(svc.conn, clientAddr, msg)

	if err := svc.pubsub.Subscribe(chanID, handler.Messages, handler.Cancel); err != nil {
		return ErrFailedSubscription
	}
	svc.put(clientID, &handler)
	return nil
}

func (svc *adapterService) Unsubscribe(clientID string) {
	svc.remove(clientID)
}

func (svc *adapterService) Handle(clientID string) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	println("handle")
	h, ok := svc.subs[clientID]
	if ok {
		h.StoreExpired(false)
	}
}
