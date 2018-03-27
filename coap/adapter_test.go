package coap

import (
	"net"
	"testing"
	"time"

	"github.com/mainflux/mainflux"

	"github.com/go-kit/kit/log"
	"github.com/gogo/protobuf/proto"
	broker "github.com/nats-io/go-nats"
	"github.com/stretchr/testify/assert"

	gocoap "github.com/dustin/go-coap"
	mgr "github.com/mainflux/mainflux/coap/mock/manager"
	nats "github.com/mainflux/mainflux/coap/mock/nats"
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
		Payload:     []byte("Test data"),
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

func NewService() Service {
	logger := log.NewNopLogger()
	nService := nats.NewService()
	nClient := nats.NewClient(&nService)
	svc := New(logger, &nClient)
	return svc
}

func MakeConn(addr string) (*net.UDPConn, *net.UDPAddr) {
	uaddr, _ := net.ResolveUDPAddr("udp", addr)
	svcConn, _ := net.ListenUDP("udp", uaddr)
	return svcConn, uaddr
}

func TestNotify(t *testing.T) {
	logger = log.NewNopLogger()
	svcConn, uaddr := MakeConn("localhost:5683")
	assert.NotNil(t, svcConn)
	assert.NotNil(t, uaddr)
	buff := make([]byte, 1500)
	handler := obsHandle(svcConn, uaddr, &validMsg, 20)
	// Notification from NATS.
	go handler(&brokerMsg)

	// Recieve notification message.
	resp, _ := gocoap.Receive(svcConn, buff)
	t.Log(string(resp.Payload))
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
	mgrClient := mgr.New("john.doe@email.com")
	recieve := svc.Recieve(mgrClient)
	m := validMsg
	m.Code = gocoap.POST
	resp := recieve(nil, nil, &m)
	assert.Equal(t, resp.Type, gocoap.Acknowledgement)
	assert.Equal(t, resp.Code, gocoap.Changed)
}

func TestObserve(t *testing.T) {
	svc := NewService()
	mgrClient := mgr.New("john.doe@email.com")
	observe := svc.Observe(mgrClient)

	m := validMsg
	m.SetOption(gocoap.Observe, 0)
	resp := observe(nil, nil, &m)
	assert.Equal(t, m.Token, resp.Token)
	assert.Equal(t, resp.Type, gocoap.Acknowledgement)
	t.Log(resp)
}

func TestUnauthorized(t *testing.T) {
	svc := NewService()
	mgrClient := mgr.New("")
	recieve := svc.Recieve(mgrClient)
	m := validMsg
	resp := recieve(nil, nil, &m)
	t.Log(resp.Token)
	assert.Equal(t, resp.Token, validMsg.Token)
	assert.Equal(t, resp.Type, gocoap.Acknowledgement)
	assert.Equal(t, resp.Code, gocoap.Unauthorized)
	t.Log(resp)
}
