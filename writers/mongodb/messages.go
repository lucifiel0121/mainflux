//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

package mongodb

import (
	"context"

	"github.com/mongodb/mongo-go-driver/mongo"

	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/writers"
)

const collectionName string = "mainflux"

var _ writers.MessageRepository = (*mongoRepo)(nil)

type mongoRepo struct {
	db *mongo.Database
}

// Message struct is used as a MongoDB representation of Mainflux message.
type message struct {
	Channel     uint64   `bson:"channel,omitempty"`
	Publisher   uint64   `bson:"publisher,omitempty"`
	Protocol    string   `bson:"protocol,omitempty"`
	Name        string   `bson:"name,omitempty"`
	Unit        string   `bson:"unit,omitempty"`
	Value       *float64 `bson:"value,omitempty"`
	StringValue *string  `bson:"stringValue,omitempty"`
	BoolValue   *bool    `bson:"boolValue,omitempty"`
	DataValue   *string  `bson:"dataValue,omitempty"`
	ValueSum    *float64 `bson:"valueSum,omitempty"`
	Time        float64  `bson:"time,omitempty"`
	UpdateTime  float64  `bson:"updateTime,omitempty"`
	Link        string   `bson:"link,omitempty"`
}

// New returns new MongoDB writer.
func New(db *mongo.Database) writers.MessageRepository {
	return &mongoRepo{db}
}

func (repo *mongoRepo) Save(msg mainflux.Message) error {
	coll := repo.db.Collection(collectionName)
	m := message{
		Channel:    msg.Channel,
		Publisher:  msg.Publisher,
		Protocol:   msg.Protocol,
		Name:       msg.Name,
		Unit:       msg.Unit,
		Time:       msg.Time,
		UpdateTime: msg.UpdateTime,
		Link:       msg.Link,
	}
	switch msg.Values.(type) {
	case *mainflux.Message_Value:
		val := msg.GetValue()
		m.Value = &val
	case *mainflux.Message_StringValue:
		strVal := msg.GetStringValue()
		m.StringValue = &strVal
	case *mainflux.Message_DataValue:
		dataVal := msg.GetDataValue()
		m.DataValue = &dataVal
	case *mainflux.Message_BoolValue:
		boolVal := msg.GetBoolValue()
		m.BoolValue = &boolVal
	}
	if msg.GetValueSum() != nil {
		valueSum := msg.GetValueSum().Value
		m.ValueSum = &valueSum
	}

	_, err := coll.InsertOne(context.Background(), m)
	return err
}
