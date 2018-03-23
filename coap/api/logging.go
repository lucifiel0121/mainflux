package api

import (
	"time"

	"github.com/go-kit/kit/log"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap"
	broker "github.com/nats-io/go-nats"
)

var _ mainflux.MessagePublisher = (*loggingMiddleware)(nil)

type loggingMiddleware struct {
	logger log.Logger
	svc    coap.PubSub
}

// LoggingMiddleware adds logging facilities to the adapter.
func LoggingMiddleware(svc coap.PubSub, logger log.Logger) coap.PubSub {
	return &loggingMiddleware{logger, svc}
}

func (lm *loggingMiddleware) Publish(msg mainflux.RawMessage) error {
	defer func(begin time.Time) {
		lm.logger.Log(
			"method", "publish",
			"took", time.Since(begin),
		)
	}(time.Now())

	return lm.svc.Publish(msg)
}

func (lm *loggingMiddleware) Subscribe(subject string, cb broker.MsgHandler) (*broker.Subscription, error) {
	defer func(begin time.Time) {
		lm.logger.Log(
			"method", "subscribe",
			"took", time.Since(begin),
		)
	}(time.Now())
	return lm.svc.Subscribe(subject, cb)
}
