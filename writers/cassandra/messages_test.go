//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

package cassandra_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/writers/cassandra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const keyspace = "mainflux"

var (
	addr = "localhost"
	msg  = mainflux.Message{
		Channel:   1,
		Publisher: 1,
		Protocol:  "mqtt",
	}
	msgsNum     = 42
	valueFields = 6
)

func TestSave(t *testing.T) {
	session, err := cassandra.Connect([]string{addr}, keyspace)
	require.Nil(t, err, fmt.Sprintf("failed to connect to Cassandra: %s", err))

	now := time.Now().Unix()
	repo := cassandra.New(session)
	for i := 0; i < msgsNum; i++ {
		// Mix possible values as well as value sum.
		count := i % valueFields
		switch count {
		case 0:
			msg.Value = &mainflux.Message_FloatValue{5}
		case 1:
			msg.Value = &mainflux.Message_BoolValue{false}
		case 2:
			msg.Value = &mainflux.Message_StringValue{"value"}
		case 3:
			msg.Value = &mainflux.Message_DataValue{"base64data"}
		case 4:
			msg.ValueSum = nil
		case 5:
			msg.ValueSum = &mainflux.SumValue{Value: 45}
		}
		msg.Time = float64(now + int64(i))

		err = repo.Save(msg)
		assert.Nil(t, err, fmt.Sprintf("expected no error, got %s", err))
	}
}
