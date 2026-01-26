GO=/usr/local/go/bin/go

.PHONY: test
test:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out
	
.PHONY: test-setup-linux
test-setup-linux:
	$(GO) test -ginkgo.focus-file setup-linux_test.go -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events
