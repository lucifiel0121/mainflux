package mocks

import (
	"errors"
	"fmt"
	"sync"

	"github.com/mainflux/mainflux"

	"github.com/gogo/protobuf/proto"
	broker "github.com/nats-io/go-nats"
)

const prefix = "channel."

var errSubscription = errors.New("Unable to subscribe")

// NatsService represents NATS service mock implementation.
type NatsService struct {
	subs map[string][]broker.MsgHandler
	mu   sync.Mutex
}

// RegisterSub adds new handler to list of handlers.
func (svc *NatsService) RegisterSub(cid string, cb broker.MsgHandler) (*broker.Subscription, error) {
	sub := &broker.Subscription{}
	svc.mu.Lock()
	svc.subs[cid] = append(svc.subs[cid], cb)
	svc.mu.Unlock()
	return sub, nil
}

// Notify subscribed clients.
func (svc *NatsService) Notify(cid string, data []byte) {
	for _, s := range svc.subs[cid] {
		fmt.Println(cid)
		msg := &broker.Msg{
			Data: data,
			Sub:  &broker.Subscription{},
		}
		s(msg)
	}
}

// NewNatsService creates new mock NATS server.
func NewNatsService() NatsService {
	return NatsService{
		subs: make(map[string][]broker.MsgHandler),
	}
}

// NatsClient represents NATS client mock implementation.
type NatsClient struct {
	service *NatsService
	broken  bool
}

// Subscribe to mock service.
func (nc NatsClient) Subscribe(cid string, cb broker.MsgHandler) (*broker.Subscription, error) {
	if nc.broken {
		return nil, errSubscription
	}
	return nc.service.RegisterSub(cid, cb)
}

// Publish to mock service.
func (nc NatsClient) Publish(msg mainflux.RawMessage) error {
	subj := prefix + msg.Channel
	data, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	nc.service.Notify(subj, data)
	return nil
}

// NewNatsClient creates new client.
func NewNatsClient(ns *NatsService, broken bool) NatsClient {
	nc := NatsClient{
		service: ns,
		broken:  broken,
	}
	return nc
}
