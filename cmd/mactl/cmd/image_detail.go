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

var imageDetailCmd = &cobra.Command{
	Use:   "detail [image-id]",
	Short: "Show image details",
	Args:  cobra.ExactArgs(1), // 引数が1つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprint(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}

		imageId := args[0]
		byteBody, _, err := m.ShowImageById(imageId)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ShowImageById error:", err)
			return err
		}

		var data interface{}
		var image api.Image
		switch outputStyle {
		case "text":
			if err := json.Unmarshal(byteBody, &image); err != nil {
				fmt.Fprintln(os.Stderr, "Failed to Unmarshal:", err)
				return err
			}
			printImageDetails(image)
			return nil

		case "json":
			err := json.Unmarshal(byteBody, &data)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to Unmarshal:", err)
				return nil
			}
			byteJson, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to Marshal:", err)
				return nil
			}
			fmt.Println(string(byteJson))
			return nil

		case "yaml":
			var data interface{}
			if err := json.Unmarshal(byteBody, &data); err != nil {
				fmt.Fprintln(os.Stderr, "Failed to Unmarshal:", err)
				return err
			}
			yamlBytes, err := yaml.Marshal(data)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Failed to Marshal:", err)
				return err
			}
			fmt.Println(string(yamlBytes))
			return nil

		default:
			fmt.Fprintln(os.Stderr, "output style must set text/json/yaml")
			return fmt.Errorf("output style must set text/json/yaml")
		}

	},
}

func init() {
	imageCmd.AddCommand(imageDetailCmd)
}

func printImageDetails(image api.Image) {
	fmt.Println("Image Details")
	fmt.Println("Summary")
	printImageDetailField("Name", stringValue(image.Metadata, func(m *api.Metadata) *string { return m.Name }))
	printImageDetailField("Id", formatID(image.Id))
	printImageDetailField("State", formatImageStatus(image.Status))
	printImageDetailField("Type", stringValue(image.Spec, func(s *api.ImageSpec) *string { return s.Type }))
	printImageDetailField("Kind", stringValue(image.Spec, func(s *api.ImageSpec) *string { return s.Kind }))
	printImageDetailField("Size", intValue(image.Spec, func(s *api.ImageSpec) *int { return s.Size }, " GB"))
	fmt.Println()

	fmt.Println("Status")
	printImageDetailField("Message", stringValue(image.Status, func(s *api.Status) *string { return s.Message }))
	printImageDetailField("Created At", timeValue(image.Status, func(s *api.Status) *time.Time { return s.CreationTimeStamp }))
	printImageDetailField("Last Updated", timeValue(image.Status, func(s *api.Status) *time.Time { return s.LastUpdateTimeStamp }))
	fmt.Println()

	fmt.Println("Storage")
	printImageDetailField("Volume Group", stringValue(image.Spec, func(s *api.ImageSpec) *string { return s.VolumeGroup }))
	printImageDetailField("Logical Vol.", stringValue(image.Spec, func(s *api.ImageSpec) *string { return s.LogicalVolume }))
	printImageDetailField("LV Path", stringValue(image.Spec, func(s *api.ImageSpec) *string { return s.LvPath }))
	printImageDetailField("QCOW2 Path", stringValue(image.Spec, func(s *api.ImageSpec) *string { return s.Qcow2Path }))
	printImageDetailField("Source URL", stringValue(image.Spec, func(s *api.ImageSpec) *string { return s.SourceUrl }))
	fmt.Println()

	fmt.Println("Metadata")
	printImageDetailField("Node Name", stringValue(image.Metadata, func(m *api.Metadata) *string { return m.NodeName }))
	printImageDetailField("UUID", stringValue(image.Metadata, func(m *api.Metadata) *string { return m.Uuid }))
	printImageDetailField("Comment", stringValue(image.Metadata, func(m *api.Metadata) *string { return m.Comment }))
}

func printImageDetailField(label, value string) {
	fmt.Printf("  %-13s %s\n", label+":", value)
}

func formatImageStatus(status *api.Status) string {
	if status == nil {
		return "N/A"
	}
	statusText := db.ImageStatus[status.StatusCode]
	if statusText == "" && status.Status != nil {
		statusText = *status.Status
	}
	if statusText == "" {
		statusText = fmt.Sprintf("UNKNOWN(%d)", status.StatusCode)
	}
	return fmt.Sprintf("%s (%d)", statusText, status.StatusCode)
}
