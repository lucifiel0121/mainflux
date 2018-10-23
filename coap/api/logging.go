//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

// +build !test

package api

import (
	"fmt"
	"net"
	"time"

	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap"
	log "github.com/mainflux/mainflux/logger"
)

var _ coap.Service = (*loggingMiddleware)(nil)

type loggingMiddleware struct {
	logger log.Logger
	svc    coap.Service
}

// LoggingMiddleware adds logging facilities to the adapter.
func LoggingMiddleware(svc coap.Service, logger log.Logger) coap.Service {
	return &loggingMiddleware{logger, svc}
}

func (lm *loggingMiddleware) Publish(msg mainflux.RawMessage) (err error) {
	defer func(begin time.Time) {
		message := fmt.Sprintf("Method publish to channel %d took %s to complete", msg.Channel, time.Since(begin))
		if err != nil {
			lm.logger.Warn(fmt.Sprintf("%s with error: %s.", message, err))
			return
		}
		lm.logger.Info(fmt.Sprintf("%s without errors.", message))
	}(time.Now())

	return lm.svc.Publish(msg)
}

func (lm *loggingMiddleware) Subscribe(chanID uint64, clientID string, clientAddr *net.UDPAddr, msg *gocoap.Message) (err error) {
	defer func(begin time.Time) {
		message := fmt.Sprintf("Method subscribe to channel %d for client %s took %s to complete", chanID, clientID, time.Since(begin))
		if err != nil {
			lm.logger.Warn(fmt.Sprintf("%s with error: %s.", message, err))
			return
		}
		lm.logger.Info(fmt.Sprintf("%s without errors.", message))
	}(time.Now())

	return lm.svc.Subscribe(chanID, clientID, clientAddr, msg)
}

func (lm *loggingMiddleware) Unsubscribe(clientID string) {
	defer func(begin time.Time) {
		message := fmt.Sprintf("Method unsubscribe for the client %s took %s to complete", clientID, time.Since(begin))
		lm.logger.Info(fmt.Sprintf("%s without errors.", message))
	}(time.Now())

	lm.svc.Unsubscribe(clientID)
}

func (lm *loggingMiddleware) Handle(clientID string) {
	defer func(begin time.Time) {
		message := fmt.Sprintf("Method handle for the client %s took %s to complete", clientID, time.Since(begin))
		lm.logger.Info(fmt.Sprintf("%s without errors.", message))
	}(time.Now())

	lm.svc.Handle(clientID)
}
