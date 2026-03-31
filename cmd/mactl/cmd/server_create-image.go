package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

var serverCreateImageCmd = &cobra.Command{
	Use:   "createimage server-id image-name",
	Short: "create image from running server",
	Args:  cobra.MinimumNArgs(2), // Idの列挙を許容
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}

		serverId := args[0]
		imageName := args[1]
		byteBody, _, err := m.MakeImageEntryFromRunningVMById(serverId, imageName)
		if err != nil {
			println("MakeImageEntryFromRunningVMById", "err", err)
			return err
		}

		switch outputStyle {
		case "text":
			var data interface{}
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			fmt.Println("サーバーのイメージ作成が受け付けられました。ID:", data.(map[string]interface{})["id"])
			return nil

		case "json":
			fmt.Println(string(byteBody))
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

		return nil
	},
}

func init() {
	serverCmd.AddCommand(serverCreateImageCmd)
}
