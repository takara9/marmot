GO=/usr/local/go/bin/go

.PHONY: test
test:
	$(GO) test . -run TestUtil -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -timeout=20m -coverprofile cover.out

.PHONY: test-unit
test-unit:
	$(GO) test . -run TestUtilIdentity -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.label-filter='unit && !integration' -timeout=20m

.PHONY: test-integration
test-integration:
	$(GO) test . -run TestUtil -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.label-filter='integration && requires-root' -timeout=20m
	
.PHONY: test-setup-linux
test-setup-linux:
	$(GO) test -ginkgo.focus-file setup-linux_test.go -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -timeout=20m
