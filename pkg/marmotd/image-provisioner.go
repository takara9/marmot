package marmotd

import (
	"log/slog"
	"strings"
	"time"

	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

// ProvisionOSImages provisions OS images from marmotd.json configuration
// Called at marmotd startup to automatically download and register configured images
func ProvisionOSImages(m *Marmot, osImages []OSImage) error {
	if len(osImages) == 0 {
		slog.Debug("No OS images configured for provisioning")
		return nil
	}

	slog.Debug("Starting OS image provisioning", "count", len(osImages))

	// Get list of existing images
	existingImages, err := m.GetImagesManage()
	if err != nil {
		slog.Error("Failed to get existing images", "err", err)
		return err
	}

	existingImageNames := make(map[string]bool)
	for _, img := range existingImages {
		existingImageNames[strings.TrimSpace(img.Metadata.Name)] = true
	}

	// Process each configured image sequentially (to avoid high load)
	for _, osImage := range osImages {
		imageName := strings.TrimSpace(osImage.Name)
		if imageName == "" {
			slog.Warn("OS image configuration has empty name, skipping")
			continue
		}

		// Check if image already exists
		if existingImageNames[imageName] {
			slog.Debug("OS image already exists, skipping", "name", imageName)
			continue
		}

		// Register the image
		if err := registerOSImage(m, osImage); err != nil {
			slog.Error("Failed to register OS image", "name", imageName, "err", err)
			// Continue processing other images even if one fails
			continue
		}

		// Add small delay between registrations to avoid overload
		time.Sleep(500 * time.Millisecond)
	}

	slog.Debug("OS image provisioning completed")
	return nil
}

// registerOSImage registers a single OS image from configuration
func registerOSImage(m *Marmot, osImage OSImage) error {
	imageName := strings.TrimSpace(osImage.Name)
	imageURL := strings.TrimSpace(osImage.URL)

	if imageName == "" {
		return nil // Skip empty name
	}

	if imageURL == "" {
		slog.Warn("OS image has empty URL, skipping", "name", imageName)
		return nil
	}

	slog.Debug("Registering OS image", "name", imageName, "url", imageURL)

	// Build API spec similar to ApiCreateImage
	imageSpec := api.Image{
		ApiVersion: "v1",
		Kind:       "Image",
		Metadata: api.Metadata{
			Name: imageName,
		},
		Spec: api.ImageSpec{
			SourceUrl: util.StringPtr(imageURL),
		},
	}

	// Add OS metadata if available
	if osImage.OSName != "" {
		imageSpec.Spec.OsName = util.StringPtr(osImage.OSName)
	}
	if osImage.OSVersion != "" {
		imageSpec.Spec.OsVersion = util.StringPtr(osImage.OSVersion)
	}

	// Assign to current node
	imageSpec.Metadata.NodeName = util.StringPtr(m.NodeName)

	// Register the image in the database
	id, err := m.Db.MakeImageEntryFromSpec(imageSpec)
	if err != nil {
		slog.Error("Failed to create image entry", "name", imageName, "err", err)
		return err
	}

	slog.Debug("OS image registered successfully", "name", imageName, "id", id)
	return nil
}
