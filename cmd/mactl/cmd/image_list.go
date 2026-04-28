package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
	"go.yaml.in/yaml/v3"
)

var imageListShowAll bool

var imageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all images",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(func() error {
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
			var allData []api.Image
			if err := json.Unmarshal(byteBody, &allData); err == nil && !imageListShowAll {
				var filtered []api.Image
				for _, image := range allData {
					if image.Metadata == nil || image.Metadata.Labels == nil || db.GetFollowerSyncRole(*image.Metadata.Labels) != "follower" {
						filtered = append(filtered, image)
					}
				}
				allData = filtered
				if len(allData) == 0 {
					byteBody = []byte("null\n")
				} else {
					byteBody, _ = json.Marshal(allData)
				}
			}

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

				fmt.Printf("  %2s  %-8s  %-16s  %-12s  %-12s  %-7s  %-4s  %-5s  %-25s\n", "No", "IMAGE-ID", "IMAGE-NAME", "STATUS", "NODE-NAME", "ROLE", "LV", "QCOW2", "CREATED-AT")
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
					if image.Metadata != nil && image.Metadata.NodeName != nil && strings.TrimSpace(*image.Metadata.NodeName) != "" {
						fmt.Printf("  %-12v", *image.Metadata.NodeName)
					} else {
						fmt.Printf("  %-12v", "N/A")
					}
					role := "master"
					if image.Metadata != nil && image.Metadata.Labels != nil {
						if db.GetFollowerSyncRole(*image.Metadata.Labels) == "follower" {
							role = "replica"
						}
					}
					fmt.Printf("  %-7v", role)
					fmt.Printf("  %-4v", existsPath(image.Spec, func(s *api.ImageSpec) *string { return s.LvPath }))
					fmt.Printf("  %-5v", existsPath(image.Spec, func(s *api.ImageSpec) *string { return s.Qcow2Path }))
					fmt.Printf("  %-25v", timeValue(image.Status, func(s *api.Status) *time.Time { return s.CreationTimeStamp }))

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
		})
	},
}

func init() {
	imageCmd.AddCommand(imageListCmd)
	imageListCmd.Flags().BoolVarP(&imageListShowAll, "all", "a", false, "レプリカも含めてすべてのイメージを表示する")
}

func existsPath[T any](obj *T, selector func(*T) *string) string {
	if obj == nil {
		return "no"
	}
	value := selector(obj)
	if value == nil || strings.TrimSpace(*value) == "" {
		return "no"
	}
	return "yes"
}
