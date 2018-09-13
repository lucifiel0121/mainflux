//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

package influxdb

import (
	"strconv"
	"sync"
	"time"

	"github.com/mainflux/mainflux/writers"

	influxdata "github.com/influxdata/influxdb/client/v2"
	"github.com/mainflux/mainflux"
)

const pointName = "messages"

var _ writers.MessageRepository = (*influxRepo)(nil)

type influxRepo struct {
	client    influxdata.Client
	batch     []*influxdata.Point
	batchSize int
	mu        sync.Mutex
	tick      <-chan time.Time
	cfg       influxdata.BatchPointsConfig
}

type fields map[string]interface{}
type tags map[string]string

// New returns new InfluxDB writer.
func New(client influxdata.Client, database string, batchSize int, batchTimeout time.Duration) (writers.MessageRepository, error) {
	repo := &influxRepo{
		client: client,
		cfg: influxdata.BatchPointsConfig{
			Database: database,
		},
		batchSize: batchSize,
		batch:     []*influxdata.Point{},
	}

	repo.tick = time.NewTicker(batchTimeout).C
	go func() {
		for {
			<-repo.tick
			repo.save()
		}
	}()

	return repo, nil
}

func (repo *influxRepo) save() error {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	bp, _ := influxdata.NewBatchPoints(repo.cfg)
	bp.AddPoints(repo.batch)
	if err := repo.client.Write(bp); err != nil {
		return err
	}
	repo.batch = []*influxdata.Point{}
	return nil
}

func (repo *influxRepo) Save(msg mainflux.Message) error {
	tags, fields := repo.tagsOf(&msg), repo.fieldsOf(&msg)
	pt, err := influxdata.NewPoint(pointName, tags, fields, time.Now())
	if err != nil {
		return err
	}
	repo.add(pt)
	return nil
}

func (repo *influxRepo) add(pt *influxdata.Point) {
	repo.mu.Lock()
	repo.batch = append(repo.batch, pt)
	repo.mu.Unlock()
	if len(repo.batch)%repo.batchSize == 0 {
		repo.save()
	}
}

func (repo *influxRepo) tagsOf(msg *mainflux.Message) tags {
	time := strconv.FormatFloat(msg.Time, 'f', -1, 64)
	update := strconv.FormatFloat(msg.UpdateTime, 'f', -1, 64)
	channel := strconv.FormatUint(msg.Channel, 10)
	publisher := strconv.FormatUint(msg.Publisher, 10)
	return tags{
		"Channel":    channel,
		"Publisher":  publisher,
		"Protocol":   msg.Protocol,
		"Name":       msg.Name,
		"Unit":       msg.Unit,
		"Link":       msg.Link,
		"Time":       time,
		"UpdateTime": update,
	}
}

func (repo *influxRepo) fieldsOf(msg *mainflux.Message) fields {
	return fields{
		"Value":       msg.Value,
		"ValueSum":    msg.ValueSum,
		"BoolValue":   msg.BoolValue,
		"StringValue": msg.StringValue,
		"DataValue":   msg.DataValue,
	}
}
