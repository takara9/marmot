package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
)

var serverStartCmd = &cobra.Command{
	Use:   "start [server-id...]",
	Short: "Start one or more servers",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}

		for _, serverId := range args {
			byteBody, _, err := m.StartServerById(serverId)
			if err != nil {
				println("StartServerById", "err", err)
				continue
			}

			switch outputStyle {
			case "text":
				var data interface{}
				if err := json.Unmarshal(byteBody, &data); err != nil {
					println("Failed to Unmarshal", err)
					return err
				}
				fmt.Println("サーバーが起動されました。ID:", data.(map[string]interface{})["id"])
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
	serverCmd.AddCommand(serverStartCmd)
}
