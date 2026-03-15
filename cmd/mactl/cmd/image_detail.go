package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var imageDetailCmd = &cobra.Command{
	Use:   "detail [image-id]",
	Short: "Show image details",
	Args:  cobra.ExactArgs(1), // 引数が1つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		imageId := args[0]
		byteBody, _, err := m.ShowImageById(imageId)
		if err != nil {
			println("ShowImageById", "err", err)
			return err
		}

		var data interface{}
		var image api.Image
		switch outputStyle {
		case "text":
			if err := json.Unmarshal(byteBody, &image); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			fmt.Printf("Image Details:\n")
			fmt.Printf("  Id: %v\n", image.Id)
			fmt.Printf("  UUID: %v\n", *image.Metadata.Uuid)
			fmt.Printf("  Name: %v\n", *image.Metadata.Name)
			return nil

		case "json":
			err := json.Unmarshal(byteBody, &data)
			if err != nil {
				println("Failed to Unmarshal", err)
				return nil
			}
			byteJson, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				println("Failed to Marshal", err)
				return nil
			}
			println(string(byteJson))
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
	imageCmd.AddCommand(imageDetailCmd)
}
