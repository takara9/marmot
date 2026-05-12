package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"go.yaml.in/yaml/v3"
)

var imageCreateCmd = &cobra.Command{
	Use:   "create -f FILE.yaml",
	Short: "Create a new OS template image",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}

		var image api.Image
		err = config.ReadYamlConfig(configFilename, &image)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if image.ApiVersion == "" {
			return fmt.Errorf("apiVersion is required in the configuration")
		}
		if image.Kind == "" {
			return fmt.Errorf("kind is required in the configuration")
		}

		if image.Metadata == nil || image.Metadata.Name == nil || *image.Metadata.Name == "" {
			return fmt.Errorf("Metadata.name is required in the configuration")
		}
		if image.Spec == nil || image.Spec.SourceUrl == nil || *image.Spec.SourceUrl == "" {
			return fmt.Errorf("Spec.source_url is required in the configuration")
		}

		byteBody, _, err := m.CreateImage(image)
		if err != nil {
			println("CreateImage", "err", err)
			return err
		}

		switch outputStyle {
		case "text":
			var data any
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			serveMap := data.(map[string]any)
			fmt.Printf("イメージの作成要求が受け入れられました。ID: %v\n", serveMap["id"])
			return nil

		case "json":
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
	imageCmd.AddCommand(imageCreateCmd)
	imageCreateCmd.Flags().StringVarP(&configFilename, "configfile", "f", "", "Configuration file or raw URL for the image")
	imageCreateCmd.MarkFlagRequired("configfile")
}
