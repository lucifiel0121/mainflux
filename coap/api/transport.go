//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/go-zoo/bone"
	"github.com/mainflux/mainflux/coap"
	log "github.com/mainflux/mainflux/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mux "github.com/dereulenspiegel/coap-mux"
	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux"
)

const (
	maxPktLen = 1500
	network   = "udp"
	protocol  = "coap"
	chanID    = "id"
	// Approximately number of supported requests per second
	timestamp = int64(time.Millisecond) * 31
)

var (
	errBadRequest = errors.New("bad request")
	errBadOption  = errors.New("bad option")
	auth          mainflux.ThingsServiceClient
	logger        log.Logger
)

type handler func(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message

func notFoundHandler(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message) *gocoap.Message {
	if m.IsConfirmable() {
		return &gocoap.Message{
			Type: gocoap.Acknowledgement,
			Code: gocoap.NotFound,
		}
	}
	return nil
}

//MakeHTTPHandler creates handler for version endpoint.
func MakeHTTPHandler() http.Handler {
	b := bone.New()
	b.GetFunc("/version", mainflux.Version("CoAP"))

	return b
}

// MakeCOAPHandler creates handler for CoAP messages.
func MakeCOAPHandler(svc coap.Service, tc mainflux.ThingsServiceClient, l log.Logger, responses chan<- string) gocoap.Handler {
	auth = tc
	logger = l

	r := mux.NewRouter()
	r.Handle("/channels/{id}/messages", gocoap.FuncHandler(receive(svc))).Methods(gocoap.POST)
	r.Handle("/channels/{id}/messages", gocoap.FuncHandler(observe(svc, responses)))
	r.NotFoundHandler = gocoap.FuncHandler(notFoundHandler)

	return r
}

func authorize(msg *gocoap.Message, res *gocoap.Message, cid uint64) (uint64, error) {
	// Device Key is passed as Uri-Query parameter, which option ID is 15 (0xf).
	key, err := authKey(msg.Option(gocoap.URIQuery))
	if err != nil {
		switch err {
		case errBadOption:
			res.Code = gocoap.BadOption
		case errBadRequest:
			res.Code = gocoap.BadRequest
		}

		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	id, err := auth.CanAccess(ctx, &mainflux.AccessReq{Token: key, ChanID: cid})

	if err != nil {
		e, ok := status.FromError(err)
		if ok {
			switch e.Code() {
			case codes.PermissionDenied:
				res.Code = gocoap.Forbidden
			default:
				res.Code = gocoap.ServiceUnavailable
			}
			return 0, err
		}
		res.Code = gocoap.InternalServerError
	}
	return id.GetValue(), nil
}

func receive(svc coap.Service) handler {
	return func(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
		// By default message is NonConfirmable, so
		// NonConfirmable response is sent back.
		var res = &gocoap.Message{
			Type: gocoap.NonConfirmable,
			// According to https://tools.ietf.org/html/rfc7252#page-47: If the POST
			// succeeds but does not result in a new resource being created on the
			// server, the response SHOULD have a 2.04 (Changed) Response Code.
			Code:      gocoap.Changed,
			MessageID: msg.MessageID,
			Token:     msg.Token,
			Payload:   []byte{},
		}

		if msg.IsConfirmable() {
			res.Type = gocoap.Acknowledgement
			res.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
			if len(msg.Payload) == 0 {
				res.Code = gocoap.BadRequest
				return res
			}
		}

		channelID := mux.Var(msg, "id")

		cid, err := strconv.ParseUint(channelID, 10, 64)
		if err != nil {
			res.Code = gocoap.NotFound
			return res
		}

		publisher, err := authorize(msg, res, cid)
		if err != nil {
			res.Code = gocoap.Forbidden
			return res
		}

		rawMsg := mainflux.RawMessage{
			Channel:   cid,
			Publisher: publisher,
			Protocol:  protocol,
			Payload:   msg.Payload,
		}

		if err := svc.Publish(rawMsg); err != nil {
			res.Code = gocoap.InternalServerError
		}

		return res
	}
}

func observe(svc coap.Service, responses chan<- string) handler {
	return func(conn *net.UDPConn, addr *net.UDPAddr, msg *gocoap.Message) *gocoap.Message {
		var res *gocoap.Message
		res = &gocoap.Message{
			Type:      gocoap.Acknowledgement,
			Code:      gocoap.Content,
			MessageID: msg.MessageID,
			Token:     msg.Token,
			Payload:   []byte{},
		}
		res.SetOption(gocoap.ContentFormat, gocoap.AppJSON)

		channelID := mux.Var(msg, "id")
		cid, err := strconv.ParseUint(channelID, 10, 64)
		if err != nil {
			res.Code = gocoap.NotFound
			return res
		}

		publisher, err := authorize(msg, res, cid)
		if err != nil {
			res.Code = gocoap.Forbidden
			logger.Warn(fmt.Sprintf("Failed to authorize: %s", err))
			return res
		}

		id := fmt.Sprintf("%x-%d-%d", msg.Token, publisher, cid)

		if msg.Type == gocoap.Acknowledgement {
			responses <- id
			return nil
		}

		if value, ok := msg.Option(gocoap.Observe).(uint32); (ok && value == 1) || msg.Type == gocoap.Reset {
			svc.Unsubscribe(id)
		}

		if value, ok := msg.Option(gocoap.Observe).(uint32); ok && value == 0 {
			res.AddOption(gocoap.Observe, 1)
			handler := coap.NewHandler()
			if err := svc.Subscribe(cid, id, handler); err != nil {
				logger.Warn(fmt.Sprintf("Failed to subscribe to NATS subject: %s", err))
				res.Code = gocoap.InternalServerError
				return res
			}

			go handleMessage(conn, addr, handler, msg)
			go ping(svc, id, conn, addr, handler, msg)
			go cancel(handler)
		}
		return res
	}
}

func cancel(h *coap.Handler) {
	<-h.Cancel
	close(h.Messages)
	h.StoreExpired(true)
}

func handleMessage(conn *net.UDPConn, addr *net.UDPAddr, h *coap.Handler, msg *gocoap.Message) {
	notifyMsg := *msg
	notifyMsg.Type = gocoap.NonConfirmable
	notifyMsg.Code = gocoap.Content
	notifyMsg.RemoveOption(gocoap.URIQuery)
	for {
		msg, ok := <-h.Messages
		if !ok {
			return
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to parse received message: %s", err))
			continue
		}

		notifyMsg.Payload = payload
		notifyMsg.MessageID = h.LoadMessageID()
		buff := new(bytes.Buffer)
		now := time.Now().UnixNano() / timestamp
		if err := binary.Write(buff, binary.BigEndian, now); err != nil {
			continue
		}

		observeVal := buff.Bytes()
		notifyMsg.SetOption(gocoap.Observe, observeVal[len(observeVal)-3:])

		if err := gocoap.Transmit(conn, addr, notifyMsg); err != nil {
			logger.Warn(fmt.Sprintf("Failed to send message to observer: %s", err))
		}
	}
}

func ping(svc coap.Service, clientID string, conn *net.UDPConn, addr *net.UDPAddr, h *coap.Handler, msg *gocoap.Message) {
	pingMsg := *msg
	pingMsg.Payload = []byte{}
	pingMsg.Type = gocoap.Confirmable
	pingMsg.RemoveOption(gocoap.URIQuery)
	defer h.Ticker.Stop()
	for {
		select {
		case _, ok := <-h.Ticker.C:
			if !ok || h.LoadExpired() {
				return
			}

			h.StoreExpired(true)
			timeout := float64(coap.AckTimeout)
			logger.Info(fmt.Sprintf("Ping client %s.", clientID))
			for i := 0; i < coap.MaxRetransmit; i++ {
				pingMsg.MessageID = h.LoadMessageID()
				gocoap.Transmit(conn, addr, pingMsg)
				time.Sleep(time.Duration(timeout * coap.AckRandomFactor))
				if !h.LoadExpired() {
					break
				}
				timeout = 2 * timeout
			}

			if h.LoadExpired() {
				svc.Unsubscribe(clientID)
				return
			}
		case <-h.Cancel:
			return
		}
	}
}
