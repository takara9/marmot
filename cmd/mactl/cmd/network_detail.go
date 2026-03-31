package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var networkDetailCmd = &cobra.Command{
	Use:   "detail [network id]",
	Short: "show network details",
	Args:  cobra.MinimumNArgs(1), // 引数が1つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}
		for _, networkId := range args {
			byteBody, _, err := m.GetVirtualNetworkById(networkId)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "GetVirtualNetworkById", "Id", networkId, "err", err)
				continue
			}

			var data api.VirtualNetwork
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", "Id", networkId, "err", err)
				continue
			}

			switch outputStyle {
			case "text":
				fmt.Println("ネットワークが表示されました。Id", networkId)
				continue

			case "json":
				fmt.Println(string(byteBody))
				continue

			case "yaml":
				yamlBytes, err := yaml.Marshal(data)
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Marshal", "Id", networkId, "err", err)
					continue

				}
				fmt.Println(string(yamlBytes))
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
	networkCmd.AddCommand(networkDetailCmd)
}
