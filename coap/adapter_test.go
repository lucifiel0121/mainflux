package coap

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap/mocks"

	"math/rand"

	"github.com/go-kit/kit/log"
	"github.com/gogo/protobuf/proto"
	broker "github.com/nats-io/go-nats"
	"github.com/stretchr/testify/assert"

	gocoap "github.com/dustin/go-coap"
)

var (
	validMsg = gocoap.Message{
		Type:      gocoap.Confirmable,
		Code:      gocoap.GET,
		MessageID: 42,
		Token:     []byte("token"),
		Payload:   []byte(`{"n":"current","t":22,"v":1.10}`),
	}
	rawMessage = mainflux.RawMessage{
		Channel:     "chann",
		Publisher:   "johndoe",
		Protocol:    "coap",
		ContentType: "JSON",
		Payload:     []byte(`{"n":"current","t":22,"v":1.10}`),
	}
	brokerMsg = broker.Msg{
		Reply:   "OK",
		Subject: "id",
		Sub:     &broker.Subscription{},
	}
)

func init() {
	validMsg.SetOption(gocoap.ContentFormat, gocoap.AppJSON)
	validMsg.SetPathString("/channels/id/messages")
	validMsg.SetOption(gocoap.URIQuery, "key=123")
	data, _ := proto.Marshal(&rawMessage)
	brokerMsg.Data = data
}

func makeMessage() *gocoap.Message {
	rand.Seed(time.Now().Unix())
	return &gocoap.Message{
		Type:      gocoap.Confirmable,
		Code:      gocoap.GET,
		MessageID: uint16(rand.Intn(1000)),
		Token:     []byte("token"),
		Payload:   []byte(`{"n":"current","t":22,"v":1.10}`),
	}
}

func NewService(username string, brokenNats bool) AdapterService {
	logger := log.NewNopLogger()
	nService := mocks.NewNatsService()
	nClient := mocks.NewNatsClient(&nService, brokenNats)
	mgrClient := mocks.NewManagerClient(username)
	svc := AdapterService{
		logger: logger,
		pubSub: &nClient,
		auth:   &mgrClient,
	}
	return svc
}

func MakeConn(addr string) (*net.UDPConn, *net.UDPAddr) {
	uaddr, _ := net.ResolveUDPAddr("udp", addr)
	svcConn, _ := net.ListenUDP("udp", uaddr)
	return svcConn, uaddr
}

func NotifySuccess(t *testing.T) {
	svcConn, uaddr := MakeConn("localhost:5683")
	// buff := make([]byte, maxPktLen)
	svc := NewService("john.doe@mail.com", false)
	handler := svc.obsHandle(svcConn, uaddr, &validMsg, 100)
	// Notification from NATS.
	go handler(&brokerMsg)
	// Receive notification message.
	resp, _ := receive(svcConn)
	assert.Equal(t, gocoap.Confirmable, resp.Type)
	assert.Equal(t, gocoap.Content, resp.Code)
	assert.Equal(t, rawMessage.Payload, resp.Payload)
	m := gocoap.Message{
		Type: gocoap.Acknowledgement,
	}
	// Send Acknowledgement to service.
	gocoap.Transmit(svcConn, uaddr, m)
	// Wait for service to process acknowledgement.
	time.Sleep(200 * time.Millisecond)
	svcConn.Close()
}

func TestNotifyTimeout(t *testing.T) {
	svcConn, uaddr := MakeConn("localhost:4300")
	svc := NewService("john.doe@mail.com", false)
	handler := svc.obsHandle(svcConn, uaddr, &validMsg, 100)
	// Notification from NATS.
	go handler(&brokerMsg)
	// Receive notification message.
	resp, _ := receive(svcConn)
	assert.Equal(t, gocoap.Confirmable, resp.Type)
	assert.Equal(t, gocoap.Content, resp.Code)
	assert.Equal(t, rawMessage.Payload, resp.Payload)
	// Wait for timeout.
	time.Sleep(200 * time.Millisecond)
	buff := []byte{}
	// Timeout happened.
	_, _, err := svcConn.ReadFromUDP(buff)
	neterr, _ := err.(net.Error)
	assert.True(t, neterr.Timeout())
	svcConn.Close()
}

func TestRequests(t *testing.T) {
	svc := NewService("john.doe@mail.com", false)
	m := validMsg
	m.Code = gocoap.POST
	resp := svc.Receive(nil, nil, &m)
	assert.Equal(t, resp.Type, gocoap.Acknowledgement)
	assert.Equal(t, resp.Code, gocoap.Changed)
}

func TestObserve(t *testing.T) {
	svc := NewService("john.doe@mail.com", false)
	m := validMsg
	m.SetOption(gocoap.Observe, 0)
	resp := svc.Observe(nil, nil, &m)
	assert.Equal(t, m.Token, resp.Token)
	assert.Equal(t, resp.Type, gocoap.Acknowledgement)
}

func TestUnableToSubscribe(t *testing.T) {
	svc := NewService("john.doe@mail.com", true)
	m := validMsg
	m.SetOption(gocoap.Observe, uint32(0))
	resp := svc.Observe(nil, nil, &m)
	assert.Equal(t, m.Token, resp.Token)
	assert.Equal(t, gocoap.InternalServerError, resp.Code)
}

func TestAuthorize(t *testing.T) {
	svc := NewService("john.doe@mail.com", false)
	unauthSvc := NewService("", false)

	unauthorized := makeMessage()
	unauthorized.AddOption(gocoap.URIQuery, nil)

	valid := makeMessage()
	valid.Code = gocoap.POST
	valid.SetOption(gocoap.URIQuery, "key=123")

	invalid := makeMessage()
	invalid.Code = gocoap.POST
	invalid.SetOption(gocoap.URIQuery, "token=34324")

	cases := map[string]struct {
		svc  AdapterService
		msg  *gocoap.Message
		code gocoap.COAPCode
	}{
		"valid request":                   {svc, valid, gocoap.Changed},
		"unauthorized":                    {unauthSvc, valid, gocoap.Unauthorized},
		"missing authorization key":       {svc, unauthorized, gocoap.BadRequest},
		"invali authorization key format": {svc, invalid, gocoap.BadOption},
	}
	for desc, tc := range cases {
		resp := tc.svc.Receive(nil, nil, tc.msg)
		assert.Equal(t, tc.msg.MessageID, resp.MessageID, fmt.Sprintf("%s: expected %d got %d\n", desc, tc.msg.MessageID, resp.MessageID))
		assert.Equal(t, tc.code, resp.Code, fmt.Sprintf("%s: expected %s got %s\n", desc, tc.code, resp.Code))
	}
}
