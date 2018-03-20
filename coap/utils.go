package coap

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mainflux/mainflux"
	broker "github.com/nats-io/go-nats"
)

func authToken(opt interface{}) (string, error) {
	if opt == nil {
		return "", errBadRequest
	}
	val, ok := opt.(string)
	if !ok {
		return "", errBadRequest
	}
	arr := strings.Split(val, "=")
	if len(arr) != 2 || strings.ToLower(arr[0]) != "token" {
		return "", errBadRequest
	}
	return arr[1], nil
}

func convertMsg(nm *broker.Msg) (*mainflux.RawMessage, error) {
	m := mainflux.RawMessage{}
	if len(nm.Data) > 0 {
		if err := json.Unmarshal(nm.Data, &m); err != nil {
			return nil, err
		}
	}
	return &m, nil
}

// BridgeHandler functions is a handler for messages recieved via NATS.
func (ca *AdapterService) BridgeHandler(nm *broker.Msg) {
	ca.logger.Log("info", "Received a message: %s\n", string(nm.Data))
	m, err := convertMsg(nm)
	if err != nil {
		ca.logger.Log("error", fmt.Sprintf("Can't convert NATS message to RawMesage: %s", err))
		return
	}
	ca.transmit(m.Channel, m.Payload, m.Publisher)
}
