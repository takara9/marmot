package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"go.yaml.in/yaml/v3"
)

var volumeDetailCmd = &cobra.Command{
	Use:   "detail [volume id]",
	Short: "show volume details",
	Args:  cobra.MinimumNArgs(1), // 引数が1つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}
		for _, volumeId := range args {
			byteBody, _, err := m.ShowVolumeById(volumeId)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "ShowVolumeById", "Id", volumeId, "err", err)
				continue
			}

			var data api.Volume
			if err := json.Unmarshal(byteBody, &data); err != nil {
				fmt.Println("Failed to Unmarshal", "Id", volumeId, "err", err)
				continue
			}

			switch outputStyle {
			case "text":
				printVolumeDetailReport(data)
				if len(args) > 1 {
					fmt.Println()
				}
				continue

			case "json":
				byteJson, err := json.Marshal(data)
				if err != nil {
					fmt.Println("Failed to Marshal", err)
					return nil
				}
				fmt.Println(string(byteJson))
				continue

			case "yaml":
				yamlBytes, err := yaml.Marshal(data)
				if err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Marshal", "Id", volumeId, "err", err)
					continue

				}
				cmd.Print(string(yamlBytes))
				continue

			default:
				fmt.Fprintln(cmd.ErrOrStderr(), "output style must set text/json/yaml")
				continue

			}
		}
		return nil
	},
}

func init() {
	volumeCmd.AddCommand(volumeDetailCmd)
}

func printVolumeDetailReport(volume api.Volume) {
	fmt.Println("Volume Details")
	fmt.Println("Summary")
	printVolumeDetailField("Name", stringValue(volume.Metadata, func(m *api.Metadata) *string { return m.Name }))
	printVolumeDetailField("Id", formatID(api.VolumeID(volume)))
	printVolumeDetailField("State", formatVolumeStatus(volume.Status))
	printVolumeDetailField("Kind", stringValue(volume.Spec, func(s *api.VolSpec) *string { return s.Kind }))
	printVolumeDetailField("Type", stringValue(volume.Spec, func(s *api.VolSpec) *string { return s.Type }))
	printVolumeDetailField("Size", intValue(volume.Spec, func(s *api.VolSpec) *int { return s.Size }, " GB"))
	fmt.Println()

	fmt.Println("Status")
	printVolumeDetailField("Message", stringValue(volume.Status, func(s *api.Status) *string { return s.Message }))
	printVolumeDetailField("Created At", timeValue(volume.Status, func(s *api.Status) *time.Time { return s.CreationTimeStamp }))
	printVolumeDetailField("Last Updated", timeValue(volume.Status, func(s *api.Status) *time.Time { return s.LastUpdateTimeStamp }))
	fmt.Println()

	fmt.Println("Storage")
	printVolumeDetailField("Path", stringValue(volume.Spec, func(s *api.VolSpec) *string { return s.Path }))
	printVolumeDetailField("Volume Group", stringValue(volume.Spec, func(s *api.VolSpec) *string { return s.VolumeGroup }))
	printVolumeDetailField("Logical Vol.", stringValue(volume.Spec, func(s *api.VolSpec) *string { return s.LogicalVolume }))
	printVolumeDetailField("OS Variant", stringValue(volume.Spec, func(s *api.VolSpec) *string { return s.OsVariant }))
	if volume.Spec != nil && volume.Spec.Persistent != nil {
		printVolumeDetailField("Persistent", boolValue(volume.Spec.Persistent))
	} else {
		printVolumeDetailField("Persistent", "N/A")
	}
	fmt.Println()

	fmt.Println("Metadata")
	printVolumeDetailField("Node Name", stringValue(volume.Metadata, func(m *api.Metadata) *string { return m.NodeName }))
	printVolumeDetailField("UUID", stringValue(volume.Metadata, func(m *api.Metadata) *string { return m.Uuid }))
	printVolumeDetailField("Comment", stringValue(volume.Metadata, func(m *api.Metadata) *string { return m.Comment }))
	printVolumeDetailField("Key", stringValue(volume.Metadata, func(m *api.Metadata) *string { return m.Key }))
}

func printVolumeDetailField(label, value string) {
	fmt.Printf("  %-13s %s\n", label+":", value)
}

func formatVolumeStatus(status *api.Status) string {
	if status == nil {
		return "N/A"
	}
	statusText := db.VolStatus[status.StatusCode]
	if statusText == "" && status.Status != nil {
		statusText = *status.Status
	}
	if statusText == "" {
		statusText = fmt.Sprintf("UNKNOWN(%d)", status.StatusCode)
	}
	return fmt.Sprintf("%s (%d)", statusText, status.StatusCode)
}
