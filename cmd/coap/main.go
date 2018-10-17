//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	kitprometheus "github.com/go-kit/kit/metrics/prometheus"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap"
	"github.com/mainflux/mainflux/coap/api"
	"github.com/mainflux/mainflux/coap/nats"
	logger "github.com/mainflux/mainflux/logger"
	thingsapi "github.com/mainflux/mainflux/things/api/grpc"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"

	broker "github.com/nats-io/go-nats"
)

const (
	defPort      string = "5683"
	defNatsURL   string = broker.DefaultURL
	defThingsURL string = "localhost:8181"
	defLogLevel  string = "error"

	envPort      string = "MF_COAP_ADAPTER_PORT"
	envNatsURL   string = "MF_NATS_URL"
	envThingsURL string = "MF_THINGS_URL"
	envLogLevel  string = "MF_COAP_ADAPTER_LOG_LEVEL"
)

type config struct {
	Port      string
	NatsURL   string
	ThingsURL string
	LogLevel  string
}

func main() {
	cfg := loadConfig()

	logger, err := logger.New(os.Stdout, cfg.LogLevel)
	if err != nil {
		log.Fatalf(err.Error())
	}

	nc, err := broker.Connect(cfg.NatsURL)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to connect to NATS: %s", err))
		os.Exit(1)
	}
	defer nc.Close()

	conn, err := grpc.Dial(cfg.ThingsURL, grpc.WithInsecure())
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to connect to users service: %s", err))
		os.Exit(1)
	}
	defer conn.Close()

	cc := thingsapi.NewClient(conn)

	pubsub := nats.New(nc, logger)
	svc := coap.New(pubsub)
	svc = api.LoggingMiddleware(svc, logger)

	svc = api.MetricsMiddleware(
		svc,
		kitprometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: "coap_adapter",
			Subsystem: "api",
			Name:      "request_count",
			Help:      "Number of requests received.",
		}, []string{"method"}),
		kitprometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
			Namespace: "coap_adapter",
			Subsystem: "api",
			Name:      "request_latency_microseconds",
			Help:      "Total duration of requests in microseconds.",
		}, []string{"method"}),
	)

	errs := make(chan error, 2)

	go func() {
		p := fmt.Sprintf(":%s", cfg.Port)
		logger.Info(fmt.Sprintf("CoAP adapter service started, exposed port %s", cfg.Port))
		errs <- api.ListenAndServe(svc, cc, p)
	}()

	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errs <- fmt.Errorf("%s", <-c)
	}()

	err = <-errs
	logger.Error(fmt.Sprintf("CoAP adapter terminated: %s", err))
}

func loadConfig() config {
	return config{
		ThingsURL: mainflux.Env(envThingsURL, defThingsURL),
		NatsURL:   mainflux.Env(envNatsURL, defNatsURL),
		Port:      mainflux.Env(envPort, defPort),
		LogLevel:  mainflux.Env(envLogLevel, defLogLevel),
	}
}
