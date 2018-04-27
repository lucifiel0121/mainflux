// Package nats contains NATS-specific message repository implementation.
package nats

import (
	"fmt"

	"github.com/sony/gobreaker"

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
	cb *gobreaker.CircuitBreaker
}

// New instantiates NATS message pubsub.
func New(nc *broker.Conn) Service {
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

func (pubsub *natsPublisher) Publish(msg mainflux.RawMessage) error {
	data, err := proto.Marshal(&msg)
	if err != nil {
		return err
	}
	_, err = pubsub.cb.Execute(func() (interface{}, error) {
		return nil, pubsub.nc.Publish(fmt.Sprintf("%s.%s", prefix, msg.Channel), data)
	})
	return err
}

func (pubsub *natsPublisher) Subscribe(chanID string, ch chan mainflux.RawMessage) (Subscription, error) {
	var sub *broker.Subscription
	println("subscribe...")
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
	// if err != nil {
	// 	go func() {
	// 		<-channel.Closed
	// 		println("closing...")
	// 		sub.Unsubscribe()
	// 		channel.Close()
	// 	}()
	// }
	return sub, err
}
