package marmotd

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Ceph volume timeout helpers", func() {
	It("uses the runtime Ceph volume operation timeout", func() {
		orig := CurrentConfig()
		cfg := *orig
		cfg.CephVolumeOperationTimeoutSeconds = 3
		SetRuntimeConfig(&cfg)
		defer SetRuntimeConfig(orig)

		ctx, cancel := newCephVolumeOperationContext()
		defer cancel()

		Expect(contextTimeoutHint(ctx)).To(Equal(3 * time.Second))
	})
})
