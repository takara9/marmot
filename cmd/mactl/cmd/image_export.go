package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/client"
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

		localNodeName, err := getLocalNodeName(m)
		if err != nil {
			return fmt.Errorf("failed to resolve current endpoint node name: %w", err)
		}

		candidates, err := pickExportableImagesByName(images, name, localNodeName)
		if err != nil {
			return err
		}

		image := candidates[0]
		qcowBytes, err := m.DownloadImageQcow2ById(image.Metadata.Id)
		if err != nil {
			return fmt.Errorf("failed to download qcow2 for image %q: %w", image.Metadata.Id, err)
		}

		outPath := filepath.Join(".", fmt.Sprintf("marmot-machine-image-%s.tgz", sanitizeArchiveName(name)))
		if err := writeImageArchive(outPath, image, qcowBytes); err != nil {
			return fmt.Errorf("failed to write archive: %w", err)
		}

		fmt.Println(outPath)
		return nil
	},
}

func init() {
	imageCmd.AddCommand(imageExportCmd)
}

func getLocalNodeName(m *client.MarmotEndpoint) (string, error) {
	body, _, err := m.GetMarmotStatus()
	if err != nil {
		return "", err
	}
	var status api.HostStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return "", err
	}
	if status.NodeName == nil || strings.TrimSpace(*status.NodeName) == "" {
		return "", fmt.Errorf("nodeName is not available in marmot status")
	}
	return strings.TrimSpace(*status.NodeName), nil
}

func pickExportableImagesByName(images []api.Image, name, preferredNode string) ([]api.Image, error) {
	matched := make([]api.Image, 0)
	localMatched := make([]api.Image, 0)
	for _, image := range images {
		if strings.TrimSpace(image.Metadata.Name) != name {
			continue
		}
		if image.Status == nil || image.Status.StatusCode != db.IMAGE_AVAILABLE {
			continue
		}
		if image.Spec.Qcow2Path == nil || strings.TrimSpace(*image.Spec.Qcow2Path) == "" {
			continue
		}

		if preferredNode != "" {
			if image.Metadata.NodeName != nil && strings.TrimSpace(*image.Metadata.NodeName) == preferredNode {
				localMatched = append(localMatched, image)
			}
		}

		if image.Metadata.Labels != nil && db.GetFollowerSyncRole(*image.Metadata.Labels) == "follower" {
			continue
		}
		matched = append(matched, image)
	}

	if preferredNode != "" {
		if len(localMatched) == 0 {
			return nil, fmt.Errorf("exportable image not found on current endpoint node (%s): %s", preferredNode, name)
		}
		sort.SliceStable(localMatched, func(i, j int) bool {
			return creationTime(localMatched[i].Status).After(creationTime(localMatched[j].Status))
		})
		return localMatched, nil
	}

	if len(matched) == 0 {
		return nil, fmt.Errorf("exportable image not found: %s", name)
	}

	sort.SliceStable(matched, func(i, j int) bool {
		return creationTime(matched[i].Status).After(creationTime(matched[j].Status))
	})
	return matched, nil
}

func pickExportableImageByName(images []api.Image, name string) (*api.Image, error) {
	matched, err := pickExportableImagesByName(images, name, "")
	if err != nil {
		return nil, err
	}
	return &matched[0], nil
}

type imageArchiveMeta struct {
	Name      string `json:"name"`
	OsName    string `json:"osName,omitempty"`
	OsVersion string `json:"osVersion,omitempty"`
}

func writeImageArchive(outPath string, image api.Image, qcow2Bytes []byte) error {
	imageName := image.Metadata.Name
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

	meta := imageArchiveMeta{Name: imageName}
	if image.Spec.OsName != nil {
		meta.OsName = strings.TrimSpace(*image.Spec.OsName)
	}
	if image.Spec.OsVersion != nil {
		meta.OsVersion = strings.TrimSpace(*image.Spec.OsVersion)
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal image metadata: %w", err)
	}
	metaHdr := &tar.Header{
		Name: "metadata.json",
		Mode: 0644,
		Size: int64(len(metaBytes)),
	}
	if err := tw.WriteHeader(metaHdr); err != nil {
		return err
	}
	if _, err := tw.Write(metaBytes); err != nil {
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
