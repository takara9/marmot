package cmd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
)

var _ = Describe("summarizeImages", func() {
	statusAvailable := "AVAILABLE"
	statusProvisioning := "PROVISIONING"
	qcow2Path := "/var/lib/marmot/images/demo.qcow2"

	It("marks image COMPLETE when all cluster nodes have consistent entries", func() {
		node1 := "marmot1"
		node2 := "marmot2"
		node3 := "marmot3"
		images := []api.Image{
			{Metadata: api.Metadata{Name: "ubuntu", NodeName: &node1}, Status: &api.Status{Status: &statusAvailable}, Spec: api.ImageSpec{Qcow2Path: &qcow2Path}},
			{Metadata: api.Metadata{Name: "ubuntu", NodeName: &node2}, Status: &api.Status{Status: &statusAvailable}, Spec: api.ImageSpec{Qcow2Path: &qcow2Path}},
			{Metadata: api.Metadata{Name: "ubuntu", NodeName: &node3}, Status: &api.Status{Status: &statusAvailable}, Spec: api.ImageSpec{Qcow2Path: &qcow2Path}},
		}

		summaries := summarizeImages(images)
		Expect(summaries).To(HaveLen(1))
		Expect(summaries[0].name).To(Equal("ubuntu"))
		Expect(summaries[0].synced).To(Equal("COMPLETE"))
		Expect(summaries[0].status).To(Equal("AVAILABLE"))
		Expect(summaries[0].hasLV).To(Equal("no"))
		Expect(summaries[0].hasQcow2).To(Equal("yes"))
	})

	It("marks image DEGRADE when image is missing on one of the cluster nodes", func() {
		node1 := "marmot1"
		node2 := "marmot2"
		node3 := "marmot3"
		images := []api.Image{
			{Metadata: api.Metadata{Name: "ubuntu", NodeName: &node1}, Status: &api.Status{Status: &statusAvailable}, Spec: api.ImageSpec{Qcow2Path: &qcow2Path}},
			{Metadata: api.Metadata{Name: "ubuntu", NodeName: &node2}, Status: &api.Status{Status: &statusAvailable}, Spec: api.ImageSpec{Qcow2Path: &qcow2Path}},
			{Metadata: api.Metadata{Name: "other", NodeName: &node1}, Status: &api.Status{Status: &statusAvailable}, Spec: api.ImageSpec{Qcow2Path: &qcow2Path}},
			{Metadata: api.Metadata{Name: "other", NodeName: &node2}, Status: &api.Status{Status: &statusAvailable}, Spec: api.ImageSpec{Qcow2Path: &qcow2Path}},
			{Metadata: api.Metadata{Name: "other", NodeName: &node3}, Status: &api.Status{Status: &statusAvailable}, Spec: api.ImageSpec{Qcow2Path: &qcow2Path}},
		}

		summaries := summarizeImages(images)
		Expect(summaries).To(HaveLen(2))

		result := map[string]imageSummary{}
		for _, s := range summaries {
			result[s.name] = s
		}
		Expect(result["ubuntu"].synced).To(Equal("DEGRADE"))
		Expect(result["other"].synced).To(Equal("COMPLETE"))
	})

	It("marks image DEGRADE and status MIXED when per-node status differs", func() {
		node1 := "marmot1"
		node2 := "marmot2"
		images := []api.Image{
			{Metadata: api.Metadata{Name: "ubuntu", NodeName: &node1}, Status: &api.Status{Status: &statusAvailable}, Spec: api.ImageSpec{Qcow2Path: &qcow2Path}},
			{Metadata: api.Metadata{Name: "ubuntu", NodeName: &node2}, Status: &api.Status{Status: &statusProvisioning}, Spec: api.ImageSpec{Qcow2Path: &qcow2Path}},
		}

		summaries := summarizeImages(images)
		Expect(summaries).To(HaveLen(1))
		Expect(summaries[0].synced).To(Equal("DEGRADE"))
		Expect(summaries[0].status).To(Equal("MIXED"))
	})
})
