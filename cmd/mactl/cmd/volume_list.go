package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
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

			fmt.Printf("%-2v  %-6v  %-16v  %-4v  %-5v  %-8v  %-12v  %-20v\n", "No", "Id", "Name", "Kind", "Type", "Size(GB)", "Status", "Path")
			for i, v := range data {
				fmt.Printf("%2d", i+1)
				fmt.Printf("  %-6v", v.Id)
				fmt.Printf("  %-16v", *v.Metadata.Name)
				fmt.Printf("  %-4v", *v.Spec.Kind)
				fmt.Printf("  %-5v", *v.Spec.Type)
				fmt.Printf("  %-8v", *v.Spec.Size)
				fmt.Printf("  %-12v", db.VolStatus[*v.Status2.Status])
				fmt.Printf("  %-20v", *v.Spec.Path)
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
