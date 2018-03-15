package api

import (
	"net"

	mux "github.com/dereulenspiegel/coap-mux"
	gocoap "github.com/dustin/go-coap"
	"github.com/mainflux/mainflux/coap"
)

// NotFoundHandler handles erroneusly formed requests.
func NotFoundHandler(l *net.UDPConn, a *net.UDPAddr, m *gocoap.Message) *gocoap.Message {
	if m.IsConfirmable() {
		return &gocoap.Message{
			Type: gocoap.Acknowledgement,
			Code: gocoap.NotFound,
		}
	}
	return nil
}

// MakeHandler function return new CoAP server with GET, POST and NOT_FOUND handlers.
func MakeHandler(as *coap.AdapterService) gocoap.Handler {
	r := mux.NewRouter()
	r.Handle("/channels/{channel_id}/messages", gocoap.FuncHandler(as.SendMessage)).Methods(gocoap.POST)
	r.Handle("/channels/{channel_id}/messages", gocoap.FuncHandler(as.ObserveMessage)).Methods(gocoap.GET)
	r.NotFoundHandler = gocoap.FuncHandler(NotFoundHandler)
	return r
}
