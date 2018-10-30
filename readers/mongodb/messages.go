//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

package mongodb

import (
	"context"

	"github.com/mainflux/mainflux"
	"github.com/mainflux/mainflux/readers"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/mongo"
	"github.com/mongodb/mongo-go-driver/mongo/findopt"
)

const collection = "mainflux"

var _ readers.MessageRepository = (*mongoRepository)(nil)

type mongoRepository struct {
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

// New returns new MongoDB reader.
func New(db *mongo.Database) readers.MessageRepository {
	return mongoRepository{db: db}
}

func (repo mongoRepository) ReadAll(chanID, offset, limit uint64) []mainflux.Message {
	col := repo.db.Collection(collection)
	cursor, err := col.Find(context.Background(), bson.NewDocument(bson.EC.Int64("channel", int64(chanID))), findopt.Limit(int64(limit)), findopt.Skip(int64(offset)))
	if err != nil {
		return []mainflux.Message{}
	}
	defer cursor.Close(context.Background())

	messages := []mainflux.Message{}
	for cursor.Next(context.Background()) {
		var m message
		if err := cursor.Decode(&m); err != nil {
			return []mainflux.Message{}
		}

		msg := mainflux.Message{
			Channel:    m.Channel,
			Publisher:  m.Publisher,
			Protocol:   m.Protocol,
			Name:       m.Name,
			Unit:       m.Unit,
			Time:       m.Time,
			UpdateTime: m.UpdateTime,
			Link:       m.Link,
		}

		if m.Value != nil {
			msg.Values = &mainflux.Message_Value{*m.Value}
		}
		if m.StringValue != nil {
			msg.Values = &mainflux.Message_StringValue{*m.StringValue}
		}
		if m.DataValue != nil {
			msg.Values = &mainflux.Message_DataValue{*m.DataValue}
		}
		if m.BoolValue != nil {
			msg.Values = &mainflux.Message_BoolValue{*m.BoolValue}
		}
		if m.ValueSum != nil {
			msg.ValueSum = &mainflux.Sum{Value: *m.ValueSum}
		}

		messages = append(messages, msg)
	}

	return messages
}
