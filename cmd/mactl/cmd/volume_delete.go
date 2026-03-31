package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var volumeDetailCmd = &cobra.Command{
	Use:   "detail [volume id]",
	Short: "show volume details",
	Args:  cobra.MinimumNArgs(1), // 引数が1つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}
		for _, volumeId := range args {
			byteBody, _, err := m.ShowVolumeById(volumeId)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "ShowVolumeById", "Id", volumeId, "err", err)
				continue
			}

			switch outputStyle {
			case "text":
				var data interface{}
				if err := json.Unmarshal(byteBody, &data); err != nil {
					println("Failed to Unmarshal", "Id", volumeId, "err", err)
					continue
				}
				fmt.Println("ボリュームの詳細情報。Id", volumeId)
				continue

			case "json":
				var data api.Volume
				if err := json.Unmarshal(byteBody, &data); err != nil {
					println("Failed to Unmarshal", "Id", volumeId, "err", err)
					continue
				}
				byteJson, err := json.MarshalIndent(data, "", "  ")
				if err != nil {
					println("Failed to Marshal", err)
					return nil
				}
				println(string(byteJson))
				continue

			case "yaml":
				var data interface{}
				if err := json.Unmarshal(byteBody, &data); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Unmarshal", "Id", volumeId, "err", err)
					continue
				}
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
	volumeCmd.AddCommand(volumeDetailCmd)
}
