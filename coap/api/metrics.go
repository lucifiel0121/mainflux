package api

import (
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap"
	broker "github.com/nats-io/go-nats"
)

var _ mainflux.MessagePublisher = (*metricsMiddleware)(nil)

type metricsMiddleware struct {
	counter metrics.Counter
	latency metrics.Histogram
	svc     coap.PubSub
}

// MetricsMiddleware instruments adapter by tracking request count and latency.
func MetricsMiddleware(svc coap.PubSub, counter metrics.Counter, latency metrics.Histogram) coap.PubSub {
	return &metricsMiddleware{
		counter: counter,
		latency: latency,
		svc:     svc,
	}
}

func (mm *metricsMiddleware) Publish(msg mainflux.RawMessage) error {
	defer func(begin time.Time) {
		mm.counter.With("method", "publish").Add(1)
		mm.latency.With("method", "publish").Observe(time.Since(begin).Seconds())
	}(time.Now())

	return mm.svc.Publish(msg)
}

func (mm *metricsMiddleware) Subscribe(subject string, cb broker.MsgHandler) (*broker.Subscription, error) {
	defer func(begin time.Time) {
		mm.counter.With("method", "subscribe").Add(1)
		mm.latency.With("method", "subscribe").Observe(time.Since(begin).Seconds())
	}(time.Now())

	return mm.svc.Subscribe(subject, cb)
}
