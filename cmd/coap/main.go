package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dustin/go-coap"
	"github.com/go-kit/kit/log"
	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	adapter "github.com/mainflux/mainflux/coap"
	"github.com/mainflux/mainflux/coap/api"
	"github.com/mainflux/mainflux/coap/nats"
	stdprometheus "github.com/prometheus/client_golang/prometheus"

	broker "github.com/nats-io/go-nats"
)

const (
	port       int    = 5683
	defNatsURL string = broker.DefaultURL
	envNatsURL string = "COAP_ADAPTER_NATS_URL"
)

type config struct {
	Port    int
	NatsURL string
}

func main() {
	cfg := loadConfig()

	logger := log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)

	nc := connectToNats(cfg, logger)
	defer nc.Close()

	pub := nats.NewMessagePublisher(nc)
	svc := api.LoggingMiddleware(pub, logger)
	svc = api.MetricsMiddleware(
		svc,
		kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: "http_adapter",
			Subsystem: "api",
			Name:      "request_count",
			Help:      "Number of requests received.",
		}, []string{"method"}),
		kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
			Namespace: "http_adapter",
			Subsystem: "api",
			Name:      "request_latency_microseconds",
			Help:      "Total duration of requests in microseconds.",
		}, []string{"method"}),
	)

	ca := adapter.New(logger, svc, nc)

	nc.Subscribe("src.http", ca.BridgeHandler)
	nc.Subscribe("src.mqtt", ca.BridgeHandler)

	errs := make(chan error, 2)

	go func() {
		coapAddr := fmt.Sprintf(":%d", cfg.Port)
		logger.Log("info", fmt.Sprintf("Start CoAP server at %s", coapAddr))
		errs <- coap.ListenAndServe("udp", coapAddr, api.MakeHandler(ca))
	}()

	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errs <- fmt.Errorf("%s", <-c)
	}()

	c := <-errs
	logger.Log("terminated", fmt.Sprintf("Proces exited: %s", c.Error()))
}

func loadConfig() *config {
	return &config{
		NatsURL: env(envNatsURL, defNatsURL),
		Port:    port,
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func connectToNats(cfg *config, logger log.Logger) *broker.Conn {
	nc, err := broker.Connect(cfg.NatsURL)
	if err != nil {
		logger.Log("error", fmt.Sprintf("Failed to connect to NATS %s", err))
		os.Exit(1)
	}

	return nc
}
