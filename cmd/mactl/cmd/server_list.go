package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all servers",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		byteBody, _, err := m.GetServers()
		if err != nil {
			println("エラー応答が返されました。", "err", err)
			return nil
		}

		var data interface{}
		switch outputStyle {
		case "text":
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			for idx, server := range data.([]interface{}) {
				serverMap := server.(map[string]interface{})
				fmt.Printf("Server %d:", idx+1)
				fmt.Printf("  ID: %v", serverMap["id"])
				fmt.Printf("  Name: %v", serverMap["name"])
				fmt.Printf("  Status: %v", serverMap["status"].(int))
				fmt.Printf("  CPU: %v", serverMap["cpu"])
				fmt.Printf("  Memory: %v MB", serverMap["memory"])
				fmt.Println()
			}
			return nil

		case "json":
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			byteBody, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				println("Failed to Marshal", err)
			}
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
	serverCmd.AddCommand(serverListCmd)
}
