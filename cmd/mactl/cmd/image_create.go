package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"go.yaml.in/yaml/v3"
)

var imageCreateCmd = &cobra.Command{
	Use:   "create [Image Name] [QCOW2 Image URL]",
	Short: "Create a new OS template image",
	Args:  cobra.MinimumNArgs(2), // 引数が1つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}

		if len(args) != 2 {
			fmt.Fprintln(cmd.ErrOrStderr(), "Usage: mactl image create [Image Name] [QCOW2 Image URL]")
			return fmt.Errorf("invalid number of arguments")
		}
		var image api.Image
		var spec api.ImageSpec
		var meta api.Metadata
		image.Spec = &spec
		image.Metadata = &meta
		image.Metadata.Name = util.StringPtr(args[0])
		image.Spec.SourceUrl = util.StringPtr(args[1])

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
}
