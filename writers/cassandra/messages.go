//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

package cassandra

import (
	"github.com/gocql/gocql"
	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/writers"
)

var _ writers.MessageRepository = (*cassandraRepository)(nil)

type cassandraRepository struct {
	session *gocql.Session
}

// New instantiates Cassandra message repository.
func New(session *gocql.Session) writers.MessageRepository {
	return &cassandraRepository{session}
}

func (cr *cassandraRepository) Save(msg mainflux.Message) error {
	cql := `INSERT INTO messages (id, channel, publisher, protocol, name, unit,
			value, string_value, bool_value, data_value, value_sum, time,
			update_time, link)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	id := gocql.TimeUUID()

	var val *float64
	var strVal, dataVal *string
	var boolVal *bool
	switch msg.Values.(type) {
	case *mainflux.Message_Value:
		v := msg.GetValue()
		val = &v
	case *mainflux.Message_StringValue:
		v := msg.GetStringValue()
		strVal = &v
	case *mainflux.Message_DataValue:
		v := msg.GetDataValue()
		dataVal = &v
	case *mainflux.Message_BoolValue:
		v := msg.GetBoolValue()
		boolVal = &v
	}

	return cr.session.Query(cql, id, msg.GetChannel(), msg.GetPublisher(),
		msg.GetProtocol(), msg.GetName(), msg.GetUnit(), val,
		strVal, boolVal, dataVal, msg.GetValueSum(), msg.GetTime(), msg.GetUpdateTime(), msg.GetLink()).Exec()
}
