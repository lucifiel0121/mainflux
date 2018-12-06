//
// Copyright (c) 2018
// Mainflux
//
// SPDX-License-Identifier: Apache-2.0
//

package cli

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/docker/docker/pkg/namesgenerator"
	mfxsdk "github.com/mainflux/mainflux/sdk/go"
	"github.com/spf13/cobra"
)

func printMap(title string, m map[string]string) {
	fmt.Printf("\n%s\n", title)
	for k, v := range m {
		fmt.Printf("name: %s, id: %s\n", k, v)
	}
}

type thing struct {
	name  string
	kind  string
	id    string
	token string
}

type channel struct {
	name string
	id   string
}

func createThing(name, kind, token string) (thing, error) {
	loc, err := sdk.CreateThing(mfxsdk.Thing{Name: name, Type: kind}, token)
	if err != nil {
		return thing{}, err
	}

	// Received location header is in the format: /things/<thing_id>
	id := strings.Split(loc, "/")[2]
	t, err := sdk.Thing(id, token)
	if err != nil {
		return thing{}, err
	}

	m := thing{
		name:  name,
		kind:  kind,
		id:    id,
		token: t.Key,
	}

	return m, nil
}

func createChannel(name, token string) (channel, error) {
	loc, err := sdk.CreateChannel(mfxsdk.Channel{Name: name}, token)
	if err != nil {
		return channel{}, nil
	}

	// Received location header is in the format: /channels/<channel_id>
	id := strings.Split(loc, "/")[2]
	c := channel{
		name: name,
		id:   id,
	}

	return c, nil
}

var cmdProvision = []cobra.Command{
	cobra.Command{
		Use:   "things",
		Short: "things <things_csv> <user_token>",
		Long:  `Provisions things`,
		Run: func(cmd *cobra.Command, args []string) {
			things := []thing{}

			if len(args) != 2 {
				logUsage(cmd.Short)
				return
			}

			c, err := os.Open(args[0])
			if err != nil {
				logError(err)
				return
			}
			reader := csv.NewReader(bufio.NewReader(c))

		LoopThingCSV:
			for {
				l, err := reader.Read()
				switch err {
				case nil:
					break
				case io.EOF:
					break LoopThingCSV
				default:
					logError(err)
					return
				}

				m, err := createThing(l[0], l[1], args[1])
				if err != nil {
					logError(err)
					return
				}

				things = append(things, m)
			}

			flush(things)
		},
	},
	cobra.Command{
		Use:   "channels",
		Short: "channels <channels_csv> <user_token>",
		Long:  `Provisions channels`,
		Run: func(cmd *cobra.Command, args []string) {
			channels := []channel{}

			if len(args) != 2 {
				logUsage(cmd.Short)
				return
			}

			c, err := os.Open(args[0])
			if err != nil {
				logError(err)
				return
			}
			reader := csv.NewReader(bufio.NewReader(c))

		LoopChanCSV:
			for {
				l, err := reader.Read()
				switch err {
				case nil:
					break
				case io.EOF:
					break LoopChanCSV
				default:
					logError(err)
					return
				}

				c, err := createChannel(l[0], args[1])
				if err != nil {
					logError(err)
					return
				}

				channels = append(channels, c)
			}

			flush(channels)
		},
	},
	cobra.Command{
		Use:   "test",
		Short: "test",
		Long: `Provisions test setup: one test user, two things and two channels. \
						Connect both things to one of the channels, \
						and only on thing to other channel.`,
		Run: func(cmd *cobra.Command, args []string) {
			nbThings := 2
			nbChan := 2
			things := []thing{}
			channels := []channel{}

			if len(args) != 0 {
				logUsage(cmd.Short)
				return
			}

			un := fmt.Sprintf("%s@email.com", namesgenerator.GetRandomName(0))
			// Create test user
			user := mfxsdk.User{
				Email:    un,
				Password: "123",
			}
			if err := sdk.CreateUser(user); err != nil {
				logError(err)
				return
			}

			ut, err := sdk.CreateToken(user)
			if err != nil {
				logError(err)
				return
			}

			// Create things
			for i := 0; i < nbThings; i++ {
				n := "d" + strconv.Itoa(i)
				k := "device"
				if i%2 != 0 {
					k = "app"
				}
				m, err := createThing(n, k, ut)
				if err != nil {
					logError(err)
					return
				}

				things = append(things, m)
			}
			// Create channels
			for i := 0; i < nbChan; i++ {
				n := "c" + strconv.Itoa(i)
				c, err := createChannel(n, ut)
				if err != nil {
					logError(err)
					return
				}

				channels = append(channels, c)
			}

			// Connect things to channels - first thing to both channels, second only to first
			for i := 0; i < nbThings; i++ {
				if err := sdk.ConnectThing(things[i].id, channels[i].id, ut); err != nil {
					logError(err)
					return
				}

				if i%2 == 0 {
					if i+1 >= len(channels) {
						break
					}
					if err := sdk.ConnectThing(things[i].id, channels[i+1].id, ut); err != nil {
						logError(err)
						return
					}
				}
			}

			flush(user)
			flush(ut)
			flush(things)
			flush(channels)
		},
	},
}

// NewProvisionCmd returns provision command.
func NewProvisionCmd() *cobra.Command {
	cmd := cobra.Command{
		Use:   "provision",
		Short: "Provision things and channels from config file",
		Long:  `Provision things and channels: use csv config file to provision things and channels`,
	}

	for i := range cmdProvision {
		cmd.AddCommand(&cmdProvision[i])
	}

	return &cmd
}
