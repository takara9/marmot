package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var serverUpdateCmd = &cobra.Command{
	Use:   "update [server-id]",
	Short: "Update a server",
	RunE: func(cmd *cobra.Command, args []string) error {
		var spec api.Server
		var meta api.Metadata
		serverId := args[0]
		spec.Id = serverId
		meta.Name = &serverName
		spec.Metadata = &meta

		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}

		byteBody, _, err := m.UpdateServerById(serverId, spec)
		if err != nil {
			println("UpdateServerById", "err", err)
			return err
		}

		switch outputStyle {
		case "text":
			println("Not implemented for text output")
			println(string(byteBody))
			return nil

		case "json":
			println(string(byteBody))
			return nil

		case "yaml":
			var data interface{}
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			yamlBytes, err := yaml.Marshal(data)
			if err != nil {
				println("Failed to Marshal", err)
				return err
			}
			fmt.Println(string(yamlBytes))
			return nil

		default:
			fmt.Println("output style must set text/json/yaml")
			return fmt.Errorf("output style must set text/json/yaml")
		}

	},
}

func init() {
	serverCmd.AddCommand(serverUpdateCmd)
	serverUpdateCmd.Flags().StringVarP(&serverName, "name", "n", "", "New name of the server")
}
