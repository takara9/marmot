GO=/usr/local/go/bin/go

.PHONY: test
test:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out


.PHONY: test2
test2:
	$(GO) test . -ginkgo.focus-file libvirtXml2_test.go -v -race  -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events


.PHONY: test-network
test-network:
	$(GO) test . -ginkgo.focus="Networks" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events  -coverprofile cover.out
