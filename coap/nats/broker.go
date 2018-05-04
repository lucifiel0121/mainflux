// Package nats contains NATS-specific message repository implementation.
package nats

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/mainflux/mainflux"
	broker "github.com/nats-io/go-nats"
)

var _ mainflux.MessagePublisher = (*natsPublisher)(nil)

const (
	maxFailedReqs   = 3
	maxFailureRatio = 0.6
	prefix          = "channel"
)

type natsPublisher struct {
	nc *broker.Conn
}

// New instantiates NATS message pubsub.
func New(nc *broker.Conn) Service {
	return &natsPublisher{nc}
}

func (pubsub *natsPublisher) Publish(msg mainflux.RawMessage) error {
	data, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	return pubsub.nc.Publish(fmt.Sprintf("%s.%s", prefix, msg.Channel), data)
}

func (pubsub *natsPublisher) Subscribe(chanID string, ch chan mainflux.RawMessage) (Subscription, error) {
	sub, err := pubsub.nc.Subscribe(fmt.Sprintf("%s.%s", prefix, chanID), func(msg *broker.Msg) {
		if msg == nil {
			return
		}
		var rawMsg mainflux.RawMessage
		if err := proto.Unmarshal(msg.Data, &rawMsg); err != nil {
			return
		}
		ch <- rawMsg
	})
	return sub, err
}
