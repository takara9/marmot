package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var volumeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all volumes",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, byteBody, _, err := m.ListVolumes()
		if err != nil {
			println("ListVolumes", "err", err)
			return err
		}

		switch outputStyle {
		case "text":
			println("Not implemented for text output")
			volumes := json.NewDecoder(strings.NewReader(string(byteBody)))
			volumes.Token()
			fmt.Println("----------")
			for volumes.More() {
				var v api.Volume
				if err := volumes.Decode(&v); err != nil {
					println("Decode volume", "err", err)
					return err
				}
				fmt.Println("Id:", v.Id)
				fmt.Println("Key:", *v.Key)
				fmt.Println("Name:", v.Name)
				fmt.Println("Type:", *v.Type)
				fmt.Println("Kind:", *v.Kind)
				fmt.Println("Size(GB):", *v.Size)
				if v.Type != nil && *v.Type == "lvm" {
					fmt.Println("Volume Group:", *v.VolumeGroup)
					fmt.Println("Logical Volume:", *v.LogicalVolume)
				} else if v.Type != nil && *v.Type == "qcow2" {
					fmt.Println("Path:", *v.Path)
				}
				fmt.Println("----------")
			}
			volumes.Token()
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
	volumeCmd.AddCommand(volumeListCmd)
	volumeListCmd.Flags().StringVarP(&volumeType, "type", "t", "qcow2", "Type of the volume (lv, qcow2)")
	volumeListCmd.Flags().StringVarP(&volumeKind, "kind", "k", "data", "Kind of the volume (os, data)")
	volumeListCmd.Flags().BoolVarP(&templateImage, "template", "i", false, "Filter by template image")
}
