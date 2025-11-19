package cmd

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"go.yaml.in/yaml/v3"
)

//go:embed version.txt
var version string

var apiConfigFilename string

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "バージョンの表示",
	Long:  `maadm クライアントのバージョンを表示します。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Debug("===", "versionCmd is called", "===")
		m, err := config.GetClientConfig2(apiConfigFilename)
		if err != nil {
			slog.Error("version", "err", err)
			return err
		}

		JsonVersion, err := m.GetVersion()
		if err != nil {
			slog.Error("version", "err", err)
			return err
		}

		sv := string(*JsonVersion.ServerVersion)
		ver := api.Version{
			ClientVersion: version,
			ServerVersion: &sv,
		}

		switch outputStyle {
		case "text":
			fmt.Println("Server version =", sv)
			fmt.Println("Client version =", version)
			return nil
		case "json":
			textJson, err := json.MarshalIndent(ver, "", "    ")
			if err != nil {
				slog.Error("failed to marshal to JSON", "err", err)
				return err
			}
			fmt.Println(string(textJson))
			return nil
		case "yaml":
			textYaml, err := yaml.Marshal(ver)
			if err != nil {
				slog.Error("failed to marshal to YAML", "err", err)
				return err
			}
			fmt.Println(string(textYaml))
			return nil
		default:
			fmt.Println("output style must set text/json/yaml")
			return fmt.Errorf("output style must set text/json/yaml")
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.PersistentFlags().StringVar(&apiConfigFilename, "api", "", "API Endpoint URL (default is $HOME/.config_marmot)")
}
