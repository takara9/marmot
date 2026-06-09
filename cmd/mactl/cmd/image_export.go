package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/db"
)

var imageExportCmd = &cobra.Command{
	Use:   "export [image-name]",
	Short: "Export an OS image to tgz",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true

		name := strings.TrimSpace(args[0])
		if name == "" {
			return fmt.Errorf("image name is required")
		}

		m, err := getClientConfig()
		if err != nil {
			return fmt.Errorf("failed to get API client config: %w", err)
		}

		listBody, _, err := m.GetImages()
		if err != nil {
			return fmt.Errorf("failed to get images: %w", err)
		}

		var images []api.Image
		if err := json.Unmarshal(listBody, &images); err != nil {
			return fmt.Errorf("failed to parse images: %w", err)
		}

		image, err := pickExportableImageByName(images, name)
		if err != nil {
			return err
		}

		qcowBytes, err := m.DownloadImageQcow2ById(image.Metadata.Id)
		if err != nil {
			return fmt.Errorf("failed to download qcow2 for image %q: %w", image.Metadata.Id, err)
		}

		outPath := filepath.Join(".", fmt.Sprintf("marmot-machine-image-%s.tgz", sanitizeArchiveName(name)))
		if err := writeImageArchive(outPath, image.Metadata.Name, qcowBytes); err != nil {
			return fmt.Errorf("failed to write archive: %w", err)
		}

		fmt.Println(outPath)
		return nil
	},
}

func init() {
	imageCmd.AddCommand(imageExportCmd)
}

func pickExportableImageByName(images []api.Image, name string) (*api.Image, error) {
	matched := make([]api.Image, 0)
	for _, image := range images {
		if strings.TrimSpace(image.Metadata.Name) != name {
			continue
		}
		if image.Metadata.Labels != nil && db.GetFollowerSyncRole(*image.Metadata.Labels) == "follower" {
			continue
		}
		if image.Status == nil || image.Status.StatusCode != db.IMAGE_AVAILABLE {
			continue
		}
		if image.Spec.Qcow2Path == nil || strings.TrimSpace(*image.Spec.Qcow2Path) == "" {
			continue
		}
		matched = append(matched, image)
	}

	if len(matched) == 0 {
		return nil, fmt.Errorf("exportable image not found: %s", name)
	}

	best := matched[0]
	for _, image := range matched[1:] {
		if creationTime(image.Status).After(creationTime(best.Status)) {
			best = image
		}
	}
	return &best, nil
}

func writeImageArchive(outPath, imageName string, qcow2Bytes []byte) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	qcowName := fmt.Sprintf("%s.qcow2", sanitizeArchiveName(imageName))
	hdr := &tar.Header{
		Name: qcowName,
		Mode: 0644,
		Size: int64(len(qcow2Bytes)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(qcow2Bytes); err != nil {
		return err
	}

	return nil
}

func sanitizeArchiveName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "image"
	}
	mapper := func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}
	sanitized := strings.Trim(strings.Map(mapper, trimmed), "-")
	if sanitized == "" {
		return "image"
	}
	return sanitized
}
