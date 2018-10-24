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
	"sync"
	"time"

	"github.com/mainflux/mainflux"
	broker "github.com/nats-io/go-nats"
)

const (
	chanID    = "id"
	keyHeader = "key"

	// AckRandomFactor is default ACK coefficient.
	AckRandomFactor = 1.5
	// AckTimeout is the amount of time to wait for a response.
	AckTimeout = 2000 * time.Millisecond
	// MaxRetransmit is the maximum number of times a message will be retransmitted.
	MaxRetransmit = 4
	// Timestamp is approximately number of supported requests per second
	Timestamp = int64(time.Millisecond) * 3
)

var (
	errBadOption = errors.New("bad option")
	// ErrFailedMessagePublish indicates that message publishing failed.
	ErrFailedMessagePublish = errors.New("failed to publish message")

	// ErrFailedSubscription indicates that client couldn't subscribe to specified channel.
	ErrFailedSubscription = errors.New("failed to subscribe to a channel")

	// ErrFailedConnection indicates that service couldn't connect to message broker.
	ErrFailedConnection = errors.New("failed to connect to message broker")
)

// Broker represents NATS broker instance.
type Broker interface {
	mainflux.MessagePublisher

	// Subscribes to channel with specified id and adds subscription to
	// service map of subscriptions under given ID.
	Subscribe(uint64, string, *Handler) error
}

// Service specifies coap service API.
type Service interface {
	Broker
	// Unsubscribe method is used to stop observing resource.
	Unsubscribe(string)
}

var _ Service = (*adapterService)(nil)

type adapterService struct {
	pubsub Broker
	subs   map[string]*Handler
	mu     sync.Mutex
}

// New instantiates the CoAP adapter implementation.
func New(pubsub Broker, responses <-chan string) Service {
	as := &adapterService{
		pubsub: pubsub,
		subs:   make(map[string]*Handler),
		mu:     sync.Mutex{},
	}

	go as.listenResponses(responses)
	return as
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
		close(h.Cancel)
		delete(svc.subs, clientID)
	}
}

// ListenResponses method handles ACK messages received from client.
func (svc *adapterService) listenResponses(responses <-chan string) {
	for {
		id := <-responses

		h, ok := svc.get(id)
		if ok {
			h.StoreExpired(false)
		}
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

func (svc *adapterService) Subscribe(chanID uint64, clientID string, handler *Handler) error {
	// Remove entry if already exists.
	svc.remove(clientID)

	if err := svc.pubsub.Subscribe(chanID, clientID, handler); err != nil {
		return ErrFailedSubscription
	}

	svc.put(clientID, handler)
	return nil
}

func (svc *adapterService) Unsubscribe(clientID string) {
	svc.remove(clientID)
}
