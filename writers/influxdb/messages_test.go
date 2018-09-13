//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

package influxdb_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	influxdata "github.com/influxdata/influxdb/client/v2"
	"github.com/mainflux/mainflux"
	log "github.com/mainflux/mainflux/logger"
	writer "github.com/mainflux/mainflux/writers/influxdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	port          string
	testLog       = log.New(os.Stdout)
	testDB        = "test"
	saveTimeout   = 5 * time.Second
	saveBatchSize = 20
	streamsSize   = 250
	client        influxdata.Client
	q             = fmt.Sprintf("SELECT * FROM test..messages\n")
	clientCfg     = influxdata.HTTPConfig{
		Username: "test",
		Password: "test",
	}
	msg = mainflux.Message{
		Channel:     45,
		Publisher:   2580,
		Protocol:    "http",
		Name:        "test name",
		Unit:        "km",
		Value:       24,
		StringValue: "24",
		BoolValue:   false,
		DataValue:   "dataValue",
		ValueSum:    24,
		Time:        13451312,
		UpdateTime:  5456565466,
		Link:        "link",
	}
)

// This is utility function to query the database.
func queryDB(cmd string) ([][]interface{}, error) {
	q := influxdata.Query{
		Command:  cmd,
		Database: testDB,
	}
	response, err := client.Query(q)
	if err != nil {
		return nil, err
	}
	if response.Error() != nil {
		return nil, response.Error()
	}
	// There is only one query, so only one result and
	// all data are stored in the same series.
	return response.Results[0].Series[0].Values, nil
}

func TestSave(t *testing.T) {
	client, err := influxdata.NewHTTPClient(clientCfg)
	require.Nil(t, err, fmt.Sprintf("Creating new InfluxDB client expected to succeed: %s.\n", err))

	repo, err := writer.New(client, testDB, 1, saveTimeout)
	require.Nil(t, err, fmt.Sprintf("Creating new InfluxDB repo expected to succeed: %s.\n", err))

	err = repo.Save(msg)
	assert.Nil(t, err, fmt.Sprintf("Save operation expected to succeed: %s.\n", err))

	time.Sleep(1 * time.Second)
	row, err := queryDB(q)
	assert.Nil(t, err, fmt.Sprintf("Querying InfluxDB to retrieve data count expected to succeed: %s.\n", err))

	count := len(row)
	assert.Equal(t, 1, count, fmt.Sprintf("Expected to have 1 value, found %d instead.\n", count))
}

func TestBatchSave(t *testing.T) {
	client, err := influxdata.NewHTTPClient(clientCfg)
	require.Nil(t, err, fmt.Sprintf("Creating new InfluxDB client expected to succeed: %s.\n", err))

	repo, err := writer.New(client, testDB, saveBatchSize, saveTimeout)
	require.Nil(t, err, fmt.Sprintf("Creating new InfluxDB repo expected to succeed: %s.\n", err))

	for i := 0; i < streamsSize; i++ {
		repo.Save(msg)
	}

	row, err := queryDB(q)
	assert.Nil(t, err, fmt.Sprintf("Querying InfluxDB to retrieve data count expected to succeed: %s.\n", err))
	time.Sleep(time.Second)
	count := len(row)
	// Add one because of previous test.
	size := streamsSize - (streamsSize % saveBatchSize) + 1
	assert.Equal(t, size, count, fmt.Sprintf("Expected to have %d value, found %d instead.\n", size, count))

	// Sleep for `saveBatchTime` to trigger timer and check if data is saved.
	time.Sleep(saveTimeout)

	row, err = queryDB(q)
	assert.Nil(t, err, fmt.Sprintf("Querying InfluxDB to retrieve data count expected to succeed: %s.\n", err))
	count = len(row)
	// Add one because of previous test.
	size = streamsSize + 1
	assert.Equal(t, size, count, fmt.Sprintf("Expected to have %d value, found %d instead.\n", size, count))
}
