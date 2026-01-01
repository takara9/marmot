package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"go.yaml.in/yaml/v3"
)

var volumeRenameCmd = &cobra.Command{
	Use:   "rename [volume id] [new name]",
	Short: "Rename a volume",
	Args:  cobra.ExactArgs(2), // 引数が2つ必要
	RunE: func(cmd *cobra.Command, args []string) error {
		volumeId := args[0]
		newName := args[1]

		var spec api.Volume
		spec.Name = newName

		byteBody, _, err := m.UpdateVolumeById(volumeId, spec)
		if err != nil {
			println("failed to update name of volume", "err", err)
			return err
		}

		switch outputStyle {
		case "text":
			fmt.Fprintf(cmd.ErrOrStderr(), "Not implemented for text output\n")
			return nil

		case "json":
			cmd.Print(string(byteBody))
			return nil

		case "yaml":
			var data interface{}
			if err := json.Unmarshal(byteBody, &data); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Unmarshal", err)
				return err
			}
			yamlBytes, err := yaml.Marshal(data)
			if err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "Failed to Marshal", err)
				return err

			}
			cmd.Print(string(yamlBytes))
			return nil

		default:
			fmt.Fprintln(cmd.ErrOrStderr(), "output style must set text/json/yaml")
			return fmt.Errorf("output style must set text/json/yaml")
		}
	},
}

func init() {
	volumeCmd.AddCommand(volumeRenameCmd)
}
