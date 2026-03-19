package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

var serverCreateImageCmd = &cobra.Command{
	Use:   "createimage [server-id...]",
	Short: "create image from running server",
	Args:  cobra.MinimumNArgs(1), // Idの列挙を許容
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, serverId := range args {
			byteBody, _, err := m.CreateImageFromServerById(serverId)
			if err != nil {
				println("CreateImageFromServerById", "err", err)
				continue
			}

			switch outputStyle {
			case "text":
				var data interface{}
				if err := json.Unmarshal(byteBody, &data); err != nil {
					println("Failed to Unmarshal", err)
					return err
				}
				fmt.Println("サーバーのイメージ作成が受け付けられました。ID:", data.(map[string]interface{})["id"])
				continue

			case "json":
				fmt.Println(string(byteBody))
				continue

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
				continue

			default:
				fmt.Println("output style must set text/json/yaml")
				return fmt.Errorf("output style must set text/json/yaml")
			}
		}
		return nil
	},
}

func init() {
	serverCmd.AddCommand(serverCreateImageCmd)
}
