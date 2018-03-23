// Package nats contains NATS-specific message repository implementation.
package nats

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap"
	broker "github.com/nats-io/go-nats"
)

var _ mainflux.MessagePublisher = (*natsPublisher)(nil)

type natsPublisher struct {
	nc *broker.Conn
}

// New instantiates NATS message publisher.
func New(nc *broker.Conn) coap.PubSub {
	return &natsPublisher{nc}
}

func (pub *natsPublisher) Publish(msg mainflux.RawMessage) error {
	data, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("channel.%s", msg.Channel)
	return pub.nc.Publish(subject, data)
}

func (pub *natsPublisher) Subscribe(subject string, cb broker.MsgHandler) (*broker.Subscription, error) {
	return pub.nc.Subscribe(subject, cb)
}
