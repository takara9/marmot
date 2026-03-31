package cmd

import (
	"encoding/json"
	"fmt"
	"os"
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
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}
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
			fmt.Printf("  UUID: %v\n", *server.Metadata.Uuid)
			fmt.Printf("  Name: %v\n", *server.Metadata.Name)
			if server.Status.CreationTimeStamp != nil {
				tm := server.Status.CreationTimeStamp.Format(time.RFC3339)
				fmt.Printf("  Create Time: %v\n", tm)
			} else {
				fmt.Printf("  Create Time: N/A\n")
			}
			if server.Status.LastUpdateTimeStamp != nil {
				tm := time.Since(*server.Status.LastUpdateTimeStamp).Hours()
				fmt.Printf("  Running Time: %.1f hours\n", tm)
			} else {
				fmt.Printf("  Running Time: N/A\n")
			}
			fmt.Printf("  OS: %v\n", *server.Spec.OsVariant)
			fmt.Printf("  Status: %v\n", db.ServerStatus[server.Status.StatusCode])
			fmt.Printf("  CPU: %v\n", *server.Spec.Cpu)
			fmt.Printf("  Memory: %v MB\n", *server.Spec.Memory)
			fmt.Printf("  Boot Volume Path: %v\n", *server.Spec.BootVolume.Spec.Path)

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
}
