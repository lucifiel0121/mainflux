package coap

import (
	"errors"
	"sync"
	"time"

	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap/nats"
	broker "github.com/nats-io/go-nats"
)

var (
	errBadRequest    = errors.New("bad request")
	errBadOption     = errors.New("bad option")
	errEntryNotFound = errors.New("observer entry not founds")
)

// AdapterService struct represents CoAP adapter service implementation.
type adapterService struct {
	pubsub nats.Service
	subs   map[string]nats.Channel
	mu     sync.Mutex
}

// New creates new CoAP adapter service struct.
func New(pubsub nats.Service) Service {
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

func (svc *adapterService) Subscribe(chanID, clientID string, ch nats.Channel) error {
	// Remove entry if already exists.
	svc.Unsubscribe(clientID)
	if err := svc.pubsub.Subscribe(chanID, ch); err != nil {
		return ErrFailedSubscription
	}
	svc.put(clientID, ch)
	return nil
}

func (svc *adapterService) Unsubscribe(clientID string) {
	svc.remove(clientID)
}

func (svc *adapterService) SetTimeout(clientID string, timer *time.Timer) error {
	sub, ok := svc.get(clientID)
	if !ok {
		return errEntryNotFound
	}
	go func() {
		select {
		case <-sub.Timer:
			timer.Stop()
		case <-timer.C:
			timer.Stop()
			svc.Unsubscribe(clientID)
		}
	}()
	return nil
}

func (svc *adapterService) RemoveTimeout(clientID string) {
	if sub, ok := svc.get(clientID); ok {
		sub.Timer <- false
	}
}
