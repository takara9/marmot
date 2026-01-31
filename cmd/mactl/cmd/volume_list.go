package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var volumeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all volumes",
	RunE: func(cmd *cobra.Command, args []string) error {
		byteBody, _, err := m.ListVolumes()
		if err != nil {
			println("ListVolumes", "err", err)
			return err
		}

		switch outputStyle {
		case "text":
			var data []api.Volume
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}

			fmt.Printf("%-2v  %-8v  %-4v  %-5v  %-8v  %-8v  %-45v\n", "No", "Id", "Kind", "Type", "Size(MB)", "Pers.", "Path")
			for i, v := range data {
				fmt.Printf("%2d", i+1)
				fmt.Printf("  %-8v", v.Id)
				fmt.Printf("  %-4v", *v.Kind)
				fmt.Printf("  %-5v", *v.Type)
				fmt.Printf("  %-8v", *v.Size)
				if v.Persistent != nil {
					fmt.Printf("  %-8v", *v.Persistent)
				} else {
					fmt.Printf("  %-8v", "N/A")
				}
				fmt.Printf("  %-45v", *v.Path)
				fmt.Println()
			}
			return nil
		case "json":
			var data []api.Volume
			if err := json.Unmarshal(byteBody, &data); err != nil {
				println("Failed to Unmarshal", err)
				return err
			}
			jsonBytes, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				println("Failed to Marshal", err)
				return err
			}
			fmt.Println(string(jsonBytes))
			return nil

		case "yaml":
			var data []api.Volume
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
	volumeCmd.AddCommand(volumeListCmd)
	volumeListCmd.Flags().StringVarP(&volumeType, "type", "t", "qcow2", "Type of the volume (lv, qcow2)")
	volumeListCmd.Flags().StringVarP(&volumeKind, "kind", "k", "data", "Kind of the volume (os, data)")
	volumeListCmd.Flags().BoolVarP(&templateImage, "template", "i", false, "Filter by template image")
}
