package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

var serverDetailCmd = &cobra.Command{
	Use:   "detail [server-id]",
	Short: "Show server details",
	Args:  cobra.ExactArgs(1), // 引数が1つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		serverId := args[0]
		byteBody, _, err := m.GetServerById(serverId)
		if err != nil {
			println("GetServerById", "err", err)
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
	serverCmd.AddCommand(serverDetailCmd)
	//serverCreateCmd.Flags().StringVarP(&serverName, "name", "n", "", "Name of the server")
	//serverCreateCmd.Flags().StringVarP(&serverType, "type", "t", "qcow2", "Type of the server (lvm, qcow2)")
	//serverCreateCmd.Flags().StringVarP(&serverKind, "kind", "k", "data", "Kind of the server (os, data)")
	//serverCreateCmd.Flags().IntVarP(&serverSize, "size", "s", 0, "Size of the server in GB")
	//serverCreateCmd.MarkFlagRequired("name")
	//serverCreateCmd.MarkFlagRequired("type")
	//serverCreateCmd.MarkFlagRequired("kind")
}
