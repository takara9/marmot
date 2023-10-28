GO=/usr/local/go/bin/go

.PHONY: test
test:
	$(GO) mod tidy
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out
