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

		var data interface{}
		switch outputStyle {
		case "text":
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			serverMap := data.(map[string]interface{})
			fmt.Printf("Server Details:\n")
			fmt.Printf("  ID: %v\n", serverMap["id"])
			fmt.Printf("  Name: %v\n", serverMap["name"])
			fmt.Printf("  Status: %v\n", serverMap["status"])
			fmt.Printf("  CPU: %v\n", serverMap["cpu"])
			fmt.Printf("  Memory: %v MB\n", serverMap["memory"])
			/*
				for idx, disk := range serverMap["disks"].([]interface{}) {
					diskMap := disk.(map[string]interface{})
					fmt.Printf("  Disk %d:\n", idx+1)
					fmt.Printf("    ID: %v\n", diskMap["id"])
					fmt.Printf("    Size: %v GB\n", diskMap["size"])
					fmt.Printf("    Status: %v\n", diskMap["status"])
				}
				for idx, nic := range serverMap["nics"].([]interface{}) {
					nicMap := nic.(map[string]interface{})
					fmt.Printf("  NIC %d:\n", idx+1)
					fmt.Printf("    ID: %v\n", nicMap["id"])
					fmt.Printf("    MAC Address: %v\n", nicMap["mac_address"])
					fmt.Printf("    IP Address: %v\n", nicMap["ip_address"])
					fmt.Printf("    Network ID: %v\n", nicMap["network_id"])
				}
			*/
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
