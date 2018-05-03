package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kit/kit/log"
	"github.com/mainflux/mainflux"
	manager "github.com/mainflux/mainflux/manager/client"

	"github.com/mainflux/mainflux/coap"
	"github.com/mainflux/mainflux/coap/api"
	"github.com/mainflux/mainflux/coap/nats"

	broker "github.com/nats-io/go-nats"
)

const (
	defPort       int    = 5683
	defNatsURL    string = broker.DefaultURL
	defManagerURL string = "http://localhost:8180"
	envPort       string = "MF_COAP_ADAPTER_PORT"
	envNatsURL    string = "MF_NATS_URL"
	envManagerURL string = "MF_MANAGER_URL"
)

type config struct {
	ManagerURL string
	NatsURL    string
	Port       int
}

func main() {
	cfg := config{
		ManagerURL: mainflux.Env(envManagerURL, defManagerURL),
		NatsURL:    mainflux.Env(envNatsURL, defNatsURL),
		Port:       defPort,
	}

	logger := log.NewJSONLogger(log.NewSyncWriter(os.Stdout))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)

	nc, err := broker.Connect(cfg.NatsURL)
	if err != nil {
		logger.Log("error", err)
		os.Exit(1)
	}
	defer nc.Close()

	pub := nats.New(nc)

	mgr := manager.NewClient(cfg.ManagerURL)
	svc := coap.New(pub)
	errs := make(chan error, 2)

	go func() {
		coapAddr := fmt.Sprintf(":%d", cfg.Port)
		logger.Log("message", fmt.Sprintf("CoAP adapter service started, exposed port %d", cfg.Port))
		errs <- api.ListenAndServe(svc, mgr, coapAddr, api.MakeHandler(svc))
	}()

	go func() {
		c := make(chan os.Signal)
		signal.Notify(c, syscall.SIGINT)
		errs <- fmt.Errorf("%s", <-c)
	}()

	c := <-errs
	logger.Log("terminated", fmt.Sprintf("Proces exited: %s", c.Error()))
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
