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
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	mux "github.com/dereulenspiegel/coap-mux"
	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap/nats"
	broker "github.com/nats-io/go-nats"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	responseBackoffMultiplier = 1.5

	// AckTimeout is the amount of time to wait for a response.
	AckTimeout = int(2 * time.Second)

	// MaxRetransmit is the maximum number of times a message will be retransmitted.
	MaxRetransmit = 4

	chanID    = "id"
	keyHeader = "key"
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
	Subscribe(uint64, string, nats.Channel) error

	// Unsubscribe method is used to stop observing resource.
	Unsubscribe(string)

	// SetTimeout sets timeout to wait CONF messages.
	SetTimeout(string, *time.Timer, int) (chan bool, error)

	// RemoveTimeout removes timeout when ACK message is received from client
	// if timeout existed.
	RemoveTimeout(string)
}

var _ Service = (*adapterService)(nil)

type adapterService struct {
	pubsub nats.Service
	subs   map[string]nats.Channel
	mu     sync.Mutex
	auth   mainflux.ThingsServiceClient
}

// New instantiates the CoAP adapter implementation.
func New(pubsub nats.Service, auth mainflux.ThingsServiceClient) Service {
	return &adapterService{
		pubsub: pubsub,
		subs:   make(map[string]nats.Channel),
		mu:     sync.Mutex{},
	}
}

func (svc *adapterService) get(clientID string) (nats.Channel, bool) {
	svc.mu.Lock()
	obs, ok := svc.subs[clientID]
	svc.mu.Unlock()
	return obs, ok
}

func (svc *adapterService) put(clientID string, obs nats.Channel) {
	svc.mu.Lock()
	svc.subs[clientID] = obs
	svc.mu.Unlock()
}

func (svc *adapterService) remove(clientID string) {
	svc.mu.Lock()
	obs, ok := svc.subs[clientID]
	if ok {
		obs.Closed <- true
		delete(svc.subs, clientID)
	}
	svc.mu.Unlock()
}

func (svc *adapterService) authorize(msg *gocoap.Message, res *gocoap.Message, cid uint64) (uint64, error) {
	// Device Key is passed as Uri-Query parameter, which option ID is 15 (0xf).
	key, err := authKey(msg.Option(gocoap.URIQuery))
	if err != nil {
		if err == errBadOption {
			res.Code = gocoap.BadOption
		}
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	id, err := svc.auth.CanAccess(ctx, &mainflux.AccessReq{Token: key, ChanID: cid})

	if err != nil {
		e, ok := status.FromError(err)
		if ok {
			switch e.Code() {
			case codes.PermissionDenied:
				res.Code = gocoap.Forbidden
			default:
				res.Code = gocoap.ServiceUnavailable
			}
			return 0, err
		}
		res.Code = gocoap.InternalServerError
	}
	return id.GetValue(), nil
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

func (svc *adapterService) Subscribe(chanID uint64, clientID string, ch nats.Channel) error {
	// Remove entry if already exists.
	svc.remove(clientID)
	if err := svc.pubsub.Subscribe(chanID, ch); err != nil {
		return ErrFailedSubscription
	}
	svc.put(clientID, ch)
	return nil
}

func (svc *adapterService) Unsubscribe(clientID string) {
	svc.remove(clientID)
}

func (svc *adapterService) SetTimeout(clientID string, timer *time.Timer, duration int) (chan bool, error) {
	sub, ok := svc.get(clientID)
	if !ok {
		return nil, errors.New("observer entry not found")
	}
	go func() {
		for {
			select {
			case _, ok := <-sub.Timer:
				timer.Stop()
				if ok {
					sub.Notify <- false
				}
				return
			case <-timer.C:
				duration *= 2
				if duration >= maxTimeout {
					timer.Stop()
					sub.Notify <- false
					svc.Unsubscribe(clientID)
					return
				}
				timer.Reset(time.Duration(duration))
				sub.Notify <- true
			}
		}
	}()
	return sub.Notify, nil
}

func (svc *adapterService) RemoveTimeout(clientID string) {
	if sub, ok := svc.get(clientID); ok {
		sub.Timer <- false
	}
}

func (svc *adapterService) Handle(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
	res := &gocoap.Message{
		Type:      gocoap.NonConfirmable,
		Code:      gocoap.Content,
		MessageID: msg.MessageID,
		Token:     msg.Token,
		Payload:   []byte{},
	}

	switch msg.Type {
	case gocoap.Reset:
		if len(msg.Payload) != 0 {
			res.Code = gocoap.BadRequest
			break
		}
		channelID := mux.Var(msg, chanID)
		cid, err := strconv.ParseUint(channelID, 10, 64)
		if err != nil {
			break
		}
		res.Type = gocoap.Acknowledgement
		publisher, err := svc.authorize(msg, res, cid)
		if err != nil {
			break
		}
		id := fmt.Sprintf("%d-%x", publisher, msg.Token)
		svc.RemoveTimeout(id)
		svc.Unsubscribe(id)
	case gocoap.Acknowledgement:
		channelID := mux.Var(msg, chanID)
		cid, err := strconv.ParseUint(channelID, 10, 64)
		if err != nil {
			break
		}
		res.Type = gocoap.Acknowledgement
		publisher, err := svc.authorize(msg, res, cid)
		if err != nil {
			break
		}
		id := fmt.Sprintf("%d-%x", publisher, msg.Token)
		svc.RemoveTimeout(id)
	}
	return res
}
