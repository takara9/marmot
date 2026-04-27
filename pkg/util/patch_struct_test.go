package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/takara9/marmot/api"
	"github.com/takara9/marmot/pkg/util"
)

var _ = Describe("PatchStruct", func() {
	It("Metadata の部分更新で既存 NodeName を保持する", func() {
		oldNode := "hvc"
		oldName := "old"
		newName := "new"

		dst := api.Volume{
			Metadata: &api.Metadata{
				Name:     &oldName,
				NodeName: &oldNode,
			},
		}
		src := api.Volume{
			Metadata: &api.Metadata{
				Name: &newName,
			},
		}

		util.PatchStruct(&dst, &src)

		Expect(dst.Metadata).NotTo(BeNil())
		Expect(dst.Metadata.Name).NotTo(BeNil())
		Expect(*dst.Metadata.Name).To(Equal(newName))
		Expect(dst.Metadata.NodeName).NotTo(BeNil())
		Expect(*dst.Metadata.NodeName).To(Equal(oldNode))
	})

	It("dst 側が nil の場合はポインタ構造体を初期化して更新する", func() {
		newName := "new"

		dst := api.Volume{}
		src := api.Volume{
			Metadata: &api.Metadata{
				Name: &newName,
			},
		}

		util.PatchStruct(&dst, &src)

		Expect(dst.Metadata).NotTo(BeNil())
		Expect(dst.Metadata.Name).NotTo(BeNil())
		Expect(*dst.Metadata.Name).To(Equal(newName))
	})
})
