package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var imageImportCmd = &cobra.Command{
	Use:   "import [filename.tgz]",
	Short: "Import an OS image archive",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		input := strings.TrimSpace(args[0])
		if input == "" {
			return fmt.Errorf("archive filename is required")
		}

		absPath, err := filepath.Abs(input)
		if err != nil {
			return fmt.Errorf("failed to resolve archive path: %w", err)
		}
		if _, err := os.Stat(absPath); err != nil {
			return fmt.Errorf("failed to access archive: %w", err)
		}

		m, err := getClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get API client config: %w", err)
		}

		body, _, err := m.ImportImageArchive(absPath)
		if err != nil {
			return fmt.Errorf("failed to import image archive: %w", err)
		}

		fmt.Println(string(body))
		return nil
	},
}

func init() {
	imageCmd.AddCommand(imageImportCmd)
}
