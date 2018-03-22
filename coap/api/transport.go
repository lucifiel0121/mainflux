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
func MakeHandler(svc coap.Service) gocoap.Handler {
	r := mux.NewRouter()
	r.Handle("/channels/{id}/messages", gocoap.FuncHandler(svc.Recieve)).Methods(gocoap.POST)
	r.Handle("/channels/{id}/messages", gocoap.FuncHandler(svc.Observe)).Methods(gocoap.GET)
	r.NotFoundHandler = gocoap.FuncHandler(NotFoundHandler)
	return r
}
