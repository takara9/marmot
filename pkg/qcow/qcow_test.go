package qcow_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/pkg/qcow"
)

var _ = Describe("QCOW2 Volume", Ordered, func() {

	BeforeAll(func() {
		os.MkdirAll("testdata", 0755)
	})

	AfterAll(func() {
		os.Remove("testdata/test.qcow2")
		os.Remove("testdata/test_copy.qcow2")
		os.RemoveAll("testdata")
	})

	Describe("Manipulation QCOW2 Volume", func() {
		Context("Lifecycle of QCOW2 Volume", func() {
			It("Create QCOW2 Volume", func() {
				err := qcow.CreateQcow("testdata/test.qcow2", 1) // 1GB
				Expect(err).NotTo(HaveOccurred())
			})

			It("Check Existence of QCOW2 Volume", func() {
				err := qcow.IsExist("testdata/test.qcow2")
				Expect(err).NotTo(HaveOccurred())
			})

			It("Copy QCOW2 Volume", func() {
				err := qcow.CopyQcow("testdata/test.qcow2", "testdata/test_copy.qcow2")
				Expect(err).NotTo(HaveOccurred())
			})

			It("Create Snapshot of QCOW2 Volume", func() {
				err := qcow.CreateSnapshotQcow("testdata/test.qcow2", "snapshot1")
				Expect(err).NotTo(HaveOccurred())
			})

			It("List Snapshot of QCOW2 Volume", func() {
				snapshots, err := qcow.ListSnapshotsQcow("testdata/test.qcow2")
				Expect(err).NotTo(HaveOccurred())
				for _, snapshot := range snapshots {
					GinkgoWriter.Printf("Snapshot: %+v\n", snapshot)
				}
			})

			It("Delete Snapshot of QCOW2 Volume", func() {
				err := qcow.DeleteSnapshotQcow("testdata/test.qcow2", "snapshot1")
				Expect(err).NotTo(HaveOccurred())
			})

			It("List Snapshot of QCOW2 Volume After Deletion", func() {
				_, err := qcow.ListSnapshotsQcow("testdata/test.qcow2")
				Expect(err).To(HaveOccurred())
				//GinkgoWriter.Printf("Snapshots after deletion: %+v\n", snapshots)
			})

			It("Remove QCOW2 Volume", func() {
				err := qcow.RemoveQcow("testdata/test.qcow2")
				Expect(err).NotTo(HaveOccurred())
				_, err = os.Stat("testdata/test.qcow2")
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
