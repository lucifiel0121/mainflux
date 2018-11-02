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

	"github.com/mainflux/mainflux"
	readers "github.com/mainflux/mainflux/readers/cassandra"
	writers "github.com/mainflux/mainflux/writers/cassandra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	keyspace      = "mainflux"
	chanID        = 1
	numOfMessages = 10
	valueFields   = 6
)

var (
	addr = "localhost"
	msg  = mainflux.Message{
		Channel:   chanID,
		Publisher: 1,
		Protocol:  "mqtt",
	}
)

func TestReadAll(t *testing.T) {
	session, err := readers.Connect([]string{addr}, keyspace)
	require.Nil(t, err, fmt.Sprintf("failed to connect to Cassandra: %s", err))
	defer session.Close()
	writer := writers.New(session)

	messages := []mainflux.Message{}
	for i := 0; i < numOfMessages; i++ {
		// Mix possible values as well as value sum.
		count := i % valueFields
		switch count {
		case 0:
			msg.Values = &mainflux.Message_Value{5}
		case 1:
			msg.Values = &mainflux.Message_BoolValue{false}
		case 2:
			msg.Values = &mainflux.Message_StringValue{"value"}
		case 3:
			msg.Values = &mainflux.Message_DataValue{"base64data"}
		case 4:
			msg.ValueSum = nil
		case 5:
			msg.ValueSum = &mainflux.Sum{Value: 45}
		}

		err := writer.Save(msg)
		require.Nil(t, err, fmt.Sprintf("failed to store message to Cassandra: %s", err))
		messages = append(messages, msg)
	}

	reader := readers.New(session)

	// Since messages are not saved in natural order,
	// the easiest way is to take all of the saved
	// data, but in different use-cases.
	cases := map[string]struct {
		chanID   uint64
		offset   uint64
		limit    uint64
		messages []mainflux.Message
	}{
		"read message page for existing channel": {
			chanID:   chanID,
			offset:   0,
			limit:    10,
			messages: messages,
		},
		"read message page for non-existent channel": {
			chanID:   2,
			offset:   0,
			limit:    10,
			messages: []mainflux.Message{},
		},
		"read message last page": {
			chanID:   chanID,
			offset:   0,
			limit:    15,
			messages: messages,
		},
	}

	for desc, tc := range cases {
		result := reader.ReadAll(tc.chanID, tc.offset, tc.limit)
		assert.ElementsMatch(t, tc.messages, result, fmt.Sprintf("%s: expected %v got %v", desc, tc.messages, result))
	}
}
