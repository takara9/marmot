package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"go.yaml.in/yaml/v3"
)

var imageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all images",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}

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
			sort.Slice(data, func(i, j int) bool {
				return creationTime(data[i].Status).Before(creationTime(data[j].Status))
			})

			fmt.Printf("  %2s  %-8s  %-16s  %-12s  %-20s  %-40s\n", "No", "IMAGE-ID", "IMAGE-NAME", "STATUS", "LV-PATH", "QCOW2-PATH")
			for i, image := range data {
				fmt.Printf("  %2d", i+1)
				fmt.Printf("  %-8v", image.Id)
				if image.Metadata.Name != nil {
					fmt.Printf("  %-16v", *image.Metadata.Name)
				} else {
					fmt.Printf("  %-16v", "N/A")
				}
				if image.Status.Status != nil {
					fmt.Printf("  %-12v", db.ImageStatus[image.Status.StatusCode])
				} else {
					fmt.Printf("  %-12v", "N/A")
				}
				if image.Spec.LvPath != nil {
					fmt.Printf("  %-20v", *image.Spec.LvPath)
				} else {
					fmt.Printf("  %-20v", "N/A")
				}
				if image.Spec.Qcow2Path != nil {
					fmt.Printf("  %-40v", *image.Spec.Qcow2Path)
				} else {
					fmt.Printf("  %-40v", "N/A")
				}

				fmt.Println()
			}
			return nil

		case "json":
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			sort.Slice(data, func(i, j int) bool {
				return creationTime(data[i].Status).Before(creationTime(data[j].Status))
			})
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
