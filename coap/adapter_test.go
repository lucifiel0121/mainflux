package coap

import (
	"net"
	"testing"
	"time"

	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/coap/mocks"

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
		MessageID: 456,
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

func NewService() AdapterService {
	logger := log.NewNopLogger()
	nService := mocks.NewNatsService()
	nClient := mocks.NewNatsClient(&nService)
	mgrClient := mocks.NewManagerClient("john.doe@email.com")
	svc := AdapterService{
		logger: logger,
		pubSub: &nClient,
		auth:   &mgrClient,
	}
	return svc
}

func NewUnauthorize() AdapterService {
	logger := log.NewNopLogger()
	nService := mocks.NewNatsService()
	nClient := mocks.NewNatsClient(&nService)
	mgrClient := mocks.NewManagerClient("")
	return AdapterService{
		logger: logger,
		pubSub: &nClient,
		auth:   &mgrClient,
	}
}

func MakeConn(addr string) (*net.UDPConn, *net.UDPAddr) {
	uaddr, _ := net.ResolveUDPAddr("udp", addr)
	svcConn, _ := net.ListenUDP("udp", uaddr)
	return svcConn, uaddr
}

func TestNotify(t *testing.T) {
	svcConn, uaddr := MakeConn("localhost:5683")
	assert.NotNil(t, svcConn)
	assert.NotNil(t, uaddr)
	buff := make([]byte, 1500)
	svc := NewService()
	handler := svc.obsHandle(svcConn, uaddr, &validMsg, 20)
	// Notification from NATS.
	go handler(&brokerMsg)

	// Receive notification message.
	resp, _ := gocoap.Receive(svcConn, buff)
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
}

func TestRequests(t *testing.T) {
	svc := NewService()
	m := validMsg
	m.Code = gocoap.POST
	resp := svc.Receive(nil, nil, &m)
	assert.Equal(t, resp.Type, gocoap.Acknowledgement)
	assert.Equal(t, resp.Code, gocoap.Changed)
}

func TestObserve(t *testing.T) {
	svc := NewService()
	m := validMsg
	m.SetOption(gocoap.Observe, 0)
	resp := svc.Observe(nil, nil, &m)
	assert.Equal(t, m.Token, resp.Token)
	assert.Equal(t, resp.Type, gocoap.Acknowledgement)
}

func TestUnauthorized(t *testing.T) {
	svc := NewUnauthorize()
	m := validMsg
	resp := svc.Receive(nil, nil, &m)
	assert.Equal(t, resp.Token, validMsg.Token)
	assert.Equal(t, resp.Type, gocoap.Acknowledgement)
	assert.Equal(t, resp.Code, gocoap.Unauthorized)
}
