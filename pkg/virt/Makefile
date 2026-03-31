GO=/usr/local/go/bin/go

.PHONY: test
test:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -timeout=20m -coverprofile cover.out


.PHONY: test2
test2:
	$(GO) test -ginkgo.focus-file libvirtXml2_test.go -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -timeout=20m


.PHONY: test-network
test-network:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus="Networks" -timeout=20m -coverprofile cover.out
