package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("splitKubernetesEngineYAMLDocuments", func() {
	It("splits documents separated by ---", func() {
		input := []byte("apiVersion: v1\nkind: Namespace\n---\napiVersion: v1\nkind: ServiceAccount\n")
		docs := splitKubernetesEngineYAMLDocuments(input)
		Expect(docs).To(HaveLen(2))
		Expect(string(docs[0])).To(ContainSubstring("Namespace"))
		Expect(string(docs[1])).To(ContainSubstring("ServiceAccount"))
	})

	It("returns a single document when there is no --- separator", func() {
		input := []byte("apiVersion: v1\nkind: ConfigMap\n")
		docs := splitKubernetesEngineYAMLDocuments(input)
		Expect(docs).To(HaveLen(1))
		Expect(string(docs[0])).To(ContainSubstring("ConfigMap"))
	})

	It("skips empty documents", func() {
		input := []byte("---\napiVersion: v1\nkind: Secret\n---\n---\n")
		docs := splitKubernetesEngineYAMLDocuments(input)
		Expect(docs).To(HaveLen(1))
		Expect(string(docs[0])).To(ContainSubstring("Secret"))
	})

	It("returns empty slice for blank input", func() {
		docs := splitKubernetesEngineYAMLDocuments([]byte(""))
		Expect(docs).To(BeEmpty())
	})
})

var _ = Describe("applyKubernetesEngineManifestObject", func() {
	It("returns error for unsupported resource kind", func() {
		doc := []byte("apiVersion: v1\nkind: Pod\nmetadata:\n  name: test\n")
		err := applyKubernetesEngineManifestObject("ns", "/ca", "/cert", "/key", "https://127.0.0.1:6443", doc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unsupported manifest resource"))
	})

	It("returns nil for a blank/comment-only document", func() {
		doc := []byte("# comment only\n")
		err := applyKubernetesEngineManifestObject("ns", "/ca", "/cert", "/key", "https://127.0.0.1:6443", doc)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns error for a resource missing metadata.name", func() {
		doc := []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: \"\"\n")
		err := applyKubernetesEngineManifestObject("ns", "/ca", "/cert", "/key", "https://127.0.0.1:6443", doc)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing metadata.name"))
	})
})
