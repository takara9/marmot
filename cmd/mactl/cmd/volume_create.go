package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
	"go.yaml.in/yaml/v3"
)

var volumeCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new volume",
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		var volume api.Volume = api.Volume{
			Metadata: &api.Metadata{
				Name: util.StringPtr(volumeName),
			},
			Spec: &api.VolSpec{
				Type: util.StringPtr(volumeType),
				Kind: util.StringPtr(volumeKind),
				Size: util.IntPtrInt(volumeSize),
			},
		}
		if len(osName) > 0 {
			volume.Spec.OsVariant = util.StringPtr(osName)
		}

		byteBody, _, err := m.CreateVolume(volume)
		if err != nil {
			println("CreateVolume", "err", err)
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
	volumeCmd.AddCommand(volumeCreateCmd)
	volumeCreateCmd.Flags().StringVarP(&volumeName, "name", "n", "", "Name of the volume")
	volumeCreateCmd.Flags().StringVarP(&volumeType, "type", "t", "qcow2", "Type of the volume (lvm, qcow2)")
	volumeCreateCmd.Flags().StringVarP(&volumeKind, "kind", "k", "data", "Kind of the volume (os, data)")
	volumeCreateCmd.Flags().StringVarP(&osName, "osname", "l", "", "Name of the OS (required for os kind)")
	volumeCreateCmd.Flags().IntVarP(&volumeSize, "size", "s", 0, "Size of the volume in GB")
	volumeCreateCmd.MarkFlagRequired("name")
	volumeCreateCmd.MarkFlagRequired("type")
	volumeCreateCmd.MarkFlagRequired("kind")
}
