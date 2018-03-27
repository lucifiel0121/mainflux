package nats

import (
	"fmt"
	"sync"
	"testing"

	"github.com/mainflux/mainflux"

	"github.com/gogo/protobuf/proto"
	broker "github.com/nats-io/go-nats"
)

const (
	prefix = "channel."
)

var log *testing.T

// Service represents NATS service mock implementation.
type Service struct {
	subs map[string][]broker.MsgHandler
	mu   sync.Mutex
}

// RegisterSub adds new handler to list of handlers
func (svc *Service) RegisterSub(cid string, cb broker.MsgHandler) (*broker.Subscription, error) {
	sub := &broker.Subscription{}
	svc.mu.Lock()
	svc.subs[cid] = append(svc.subs[cid], cb)
	svc.mu.Unlock()
	return sub, nil
}

// Notify subscribed clients.
func (svc *Service) Notify(cid string, data []byte) {
	for _, s := range svc.subs[cid] {
		fmt.Println(cid)
		msg := &broker.Msg{
			Data: data,
			Sub:  &broker.Subscription{},
		}
		s(msg)
	}
}

// NewService creates new mock NATS server.
func NewService() Service {
	return Service{
		subs: make(map[string][]broker.MsgHandler),
	}
}

// Client represents NATS client mock implementation.
type Client struct {
	service *Service
}

// Subscribe to mock service.
func (nc Client) Subscribe(cid string, cb broker.MsgHandler) (*broker.Subscription, error) {
	return nc.service.RegisterSub(cid, cb)
}

// Publish to mock service.
func (nc Client) Publish(msg mainflux.RawMessage) error {
	subj := prefix + msg.Channel
	data, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	nc.service.Notify(subj, data)
	return nil
}

// NewClient creates new client.
func NewClient(ns *Service) Client {
	nc := Client{
		service: ns,
	}
	return nc
}
