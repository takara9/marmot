GO=/usr/local/go/bin/go

.PHONY: test
test:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out

.PHONY: test-server
test-server:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus="Servers" -coverprofile cover.out

.PHONY: test-network
test-network:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus="Networks" -coverprofile cover.out

.PHONY: test-ipam
test-ipam:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus="IPAM" -coverprofile cover.out
