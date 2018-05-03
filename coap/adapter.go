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
	subs   map[string]nats.Observer
	mu     sync.Mutex
}

// New creates new CoAP adapter service struct.
func New(pubsub nats.Service) Service {
	return &adapterService{
		pubsub: pubsub,
		subs:   make(map[string]nats.Observer),
		mu:     sync.Mutex{},
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

func (svc *adapterService) Subscribe(chanID, clientID string, ch chan mainflux.RawMessage) error {
	sub, err := svc.pubsub.Subscribe(chanID, ch)
	if err != nil {
		return ErrFailedSubscription
	}
	// Remove entry if already exists.
	if err := svc.Unsubscribe(clientID); err != nil {
		return err
	}
	svc.mu.Lock()
	svc.subs[clientID] = nats.Observer{
		Sub:     sub,
		MsgCh:   ch,
		Timeout: make(chan bool),
	}
	svc.mu.Unlock()
	return nil
}

func (svc *adapterService) Unsubscribe(id string) error {
	svc.mu.Lock()
	obs, ok := svc.subs[id]
	svc.mu.Unlock()
	if !ok {
		return nil
	}
	if err := obs.Sub.Unsubscribe(); err != nil {
		return err
	}
	svc.mu.Lock()
	close(obs.MsgCh)
	close(obs.Timeout)
	delete(svc.subs, id)
	svc.mu.Unlock()
	return nil
}

func (svc *adapterService) SetTimeout(clientID string, timeout time.Duration) error {
	svc.mu.Lock()
	sub, ok := svc.subs[clientID]
	svc.mu.Unlock()
	if !ok {
		return errEntryNotFound
	}
	timer := time.NewTimer(timeout)
	go func() {
		select {
		case <-sub.Timeout:
			timer.Stop()
		case <-timer.C:
			timer.Stop()
			svc.Unsubscribe(clientID)
		}
	}()
	return nil
}

func (svc *adapterService) RemoveTimeout(clientID string) {
	svc.mu.Lock()
	sub, ok := svc.subs[clientID]
	if ok {
		sub.Timeout <- false
	}
	svc.mu.Unlock()
}
