package cmd

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
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

		var data interface{}
		var server api.Server
		switch outputStyle {
		case "text":
			if err := json.Unmarshal(byteBody, &server); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			fmt.Printf("Server Details:\n")
			fmt.Printf("  Id: %v\n", server.Id)
			fmt.Printf("  UUID: %v\n", *server.Uuid)
			fmt.Printf("  Name: %v\n", *server.Name)
			if server.CTime != nil {
				tm := server.CTime.Format(time.RFC3339)
				fmt.Printf("  Create Time: %v\n", tm)
			} else {
				fmt.Printf("  Create Time: N/A\n")
			}
			if server.CTime != nil {
				tm := time.Since(*server.CTime).Hours()
				fmt.Printf("  Running Time: %.1f hours\n", tm)
			} else {
				fmt.Printf("  Running Time: N/A\n")
			}
			fmt.Printf("  OS: %v\n", *server.OsVariant)
			fmt.Printf("  Status: %v\n", db.ServerStatus[*server.Status])
			fmt.Printf("  CPU: %v\n", *server.Cpu)
			fmt.Printf("  Memory: %v MB\n", *server.Memory)
			fmt.Printf("  Boot Volume Path: %v\n", *server.BootVolume.Path)

			return nil

		case "json":
			err := json.Unmarshal(byteBody, &data)
			if err != nil {
				println("Failed to Unmarshal", err)
				return nil
			}
			byteJson, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				println("Failed to Marshal", err)
				return nil
			}
			println(string(byteJson))
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
