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
	"strings"

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

			c, _ := os.Open(args[0])
			reader := csv.NewReader(bufio.NewReader(c))
			for {
				l, err := reader.Read()
				if err == io.EOF {
					break
				} else if err != nil {
					logError(err)
					return
				}

				loc, err := sdk.CreateThing(mfxsdk.Thing{Name: l[1], Type: l[0]}, args[1])
				if err != nil {
					logError(err)
					return
				}

				// Received location header is in the format: /things/<thing_id>
				id := strings.Split(loc, "/")[2]
				t, err := sdk.Thing(id, args[1])
				if err != nil {
					logError(err)
					return
				}

				m := thing{
					name:  l[1],
					kind:  l[0],
					id:    id,
					token: t.Key,
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

			c, _ := os.Open(args[0])
			reader := csv.NewReader(bufio.NewReader(c))
			for {
				l, err := reader.Read()
				if err == io.EOF {
					break
				} else if err != nil {
					logError(err)
					return
				}

				loc, err := sdk.CreateChannel(mfxsdk.Channel{Name: l[0]}, args[1])
				if err != nil {
					logError(err)
					return
				}

				// Received location header is in the format: /channels/<channel_id>
				id := strings.Split(loc, "/")[2]
				c := channel{
					name: l[0],
					id:   id,
				}

				channels = append(channels, c)
			}

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
