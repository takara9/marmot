package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/config"
	"github.com/takara9/marmot/pkg/util"
	"go.yaml.in/yaml/v3"
)

type volumeCreateConfig struct {
	Name          string  `yaml:"name"`
	Comment       *string `yaml:"comment,omitempty"`
	Type          *string `yaml:"type,omitempty"`
	Kind          *string `yaml:"kind,omitempty"`
	Size          *int    `yaml:"size,omitempty"`
	OsVariant     *string `yaml:"os_variant,omitempty"`
	LogicalVolume *string `yaml:"logical_volume,omitempty"`
	Path          *string `yaml:"path,omitempty"`
	Persistent    *bool   `yaml:"persistent,omitempty"`
	VolumeGroup   *string `yaml:"volume_group,omitempty"`
}

var volumeCreateCmd = &cobra.Command{
	Use:   "create -f FILE.yaml",
	Short: "Create a new volume",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		m, err := getClientConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to get API client config:", err)
			os.Exit(1)
		}

		var conf volumeCreateConfig
		err = config.ReadYamlConfig(configFilename, &conf)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if conf.Name == "" {
			return fmt.Errorf("name is required in the configuration")
		}
		if conf.Type == nil || *conf.Type == "" {
			return fmt.Errorf("type is required in the configuration")
		}
		if conf.Kind == nil || *conf.Kind == "" {
			return fmt.Errorf("kind is required in the configuration")
		}

		var volume api.Volume = api.Volume{
			Metadata: &api.Metadata{
				Name: util.StringPtr(conf.Name),
			},
			Spec: &api.VolSpec{
				Type: util.StringPtr(*conf.Type),
				Kind: util.StringPtr(*conf.Kind),
			},
		}
		if conf.Comment != nil {
			volume.Metadata.Comment = util.StringPtr(*conf.Comment)
		}
		if conf.Size != nil {
			volume.Spec.Size = util.IntPtrInt(*conf.Size)
		}
		if conf.OsVariant != nil {
			volume.Spec.OsVariant = util.StringPtr(*conf.OsVariant)
		}
		if conf.LogicalVolume != nil {
			volume.Spec.LogicalVolume = util.StringPtr(*conf.LogicalVolume)
		}
		if conf.Path != nil {
			volume.Spec.Path = util.StringPtr(*conf.Path)
		}
		if conf.Persistent != nil {
			volume.Spec.Persistent = util.BoolPtr(*conf.Persistent)
		}
		if conf.VolumeGroup != nil {
			volume.Spec.VolumeGroup = util.StringPtr(*conf.VolumeGroup)
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
			fmt.Printf("ボリュームの作成要求が受け入れられました。ID: %v\n", serveMap["id"])
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
	volumeCreateCmd.Flags().StringVarP(&configFilename, "configfile", "f", "", "Configuration file or raw URL for the volume")
	volumeCreateCmd.MarkFlagRequired("configfile")
}
