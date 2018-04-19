// Package nats contains NATS-specific message repository implementation.
package nats

import (
	"fmt"

	"github.com/sony/gobreaker"

	"github.com/golang/protobuf/proto"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap"
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
	cb *gobreaker.CircuitBreaker
}

// New instantiates NATS message publisher.
func New(nc *broker.Conn) coap.Service {
	st := gobreaker.Settings{
		Name: "NATS",
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			fr := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= maxFailedReqs && fr >= maxFailureRatio
		},
	}
	cb := gobreaker.NewCircuitBreaker(st)
	return &natsPublisher{nc, cb}
}

func (pub *natsPublisher) Publish(msg mainflux.RawMessage) error {
	data, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	subject := fmt.Sprintf("%s.%s", prefix, msg.Channel)
	return pub.nc.Publish(subject, data)
}

func (pub *natsPublisher) Subscribe(chanID string, channel coap.Channel) error {
	var sub *broker.Subscription
	sub, err := pub.nc.Subscribe(fmt.Sprintf("%s.%s", prefix, chanID), func(msg *broker.Msg) {
		if msg == nil {
			return
		}

		var rawMsg mainflux.RawMessage
		if err := proto.Unmarshal(msg.Data, &rawMsg); err != nil {
			return
		}
		select {
		case channel.Messages <- rawMsg:
		case <-channel.Closed:
			sub.Unsubscribe()
			channel.Close()
		}
	})

	return err
}
