package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var imageCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new image",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		var image api.Image

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
			fmt.Printf("ボリュームが作成されました。 ID: %v\n", serveMap["id"])
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
	//imageCreateCmd.Flags().StringVarP(&imageName, "name", "n", "", "Name of the image")
	//imageCreateCmd.Flags().StringVarP(&imageType, "type", "t", "qcow2", "Type of the image (lvm, qcow2)")
	//imageCreateCmd.Flags().StringVarP(&imageKind, "kind", "k", "data", "Kind of the image (os, data)")
	//imageCreateCmd.Flags().StringVarP(&osName, "osname", "l", "", "Name of the OS (required for os kind)")
	//imageCreateCmd.Flags().IntVarP(&imageSize, "size", "s", 0, "Size of the image in GB")
	//imageCreateCmd.MarkFlagRequired("name")
	//imageCreateCmd.MarkFlagRequired("type")
	//volumeCreateCmd.MarkFlagRequired("kind")
}
