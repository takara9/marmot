package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"go.yaml.in/yaml/v3"
)

var imageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all images",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error

		byteBody, _, err := m.GetImages()
		if err != nil {
			println("エラー応答が返されました。", "err", err)
			return nil
		}

		//var data interface{}
		var data []api.Image
		switch outputStyle {
		case "text":
			if string(byteBody) == "null\n" {
				fmt.Println("イメージが見つかりません。")
				return nil
			}

			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}

			fmt.Printf("  %2s  %-10s  %-20s  %-12s  %-3s  %-8s  %-15s  %-15s\n", "No", "Image-ID", "Image-Name", "Status", "CPU", "RAM(MB)", "IP-Address", "Network")
			for i, image := range data {
				fmt.Printf("  %2d", i+1)
				fmt.Printf("  %-10v", image.Id)
				fmt.Printf("  %-20v", *image.Metadata.Name)
				fmt.Printf("  %-12v", db.ImageStatus[*image.Status.Status])
				fmt.Println()
			}
			return nil

		case "json":
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			byteBody, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				fmt.Println("Failed to Marshal", err)
			}
			fmt.Println(string(byteBody))
			return nil

		case "yaml":
			var data interface{}
			if err := json.Unmarshal(byteBody, &data); err != nil {
				fmt.Println("Failed to Unmarshal", err)
				return err
			}
			yamlBytes, err := yaml.Marshal(data)
			if err != nil {
				fmt.Println("Failed to Marshal", err)
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
	imageCmd.AddCommand(imageListCmd)
}
