package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var volumeDeleteCmd = &cobra.Command{
	Use:   "delete [volume id]",
	Short: "delete a volume",
	Args:  cobra.MinimumNArgs(1), // 引数が1つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}
		for _, volumeId := range args {
			byteBody, _, err := m.DeleteVolumeById(volumeId)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "DeleteVolumeById", "Id", volumeId, "err", err)
				continue
			}
			var data api.Volume
			if err := json.Unmarshal(byteBody, &data); err != nil {
				fmt.Println("Failed to Unmarshal", "Id", volumeId, "err", err)
				continue
			}

			switch outputStyle {
			case "text":
				fmt.Println("ボリュームが削除されました。Id", volumeId)
				continue

			case "json":
				cmd.Print(string(byteBody))
				continue

			case "yaml":
				yamlBytes, err := yaml.Marshal(data)
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Marshal", "Id", volumeId, "err", err)
					continue

				}
				cmd.Print(string(yamlBytes))
				continue

			default:
				fmt.Fprintln(cmd.ErrOrStderr(), "output style must set text/json/yaml")
				continue

			}
		}
		return nil
	},
}

func init() {
	volumeCmd.AddCommand(volumeDeleteCmd)
}
