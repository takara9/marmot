GO=/usr/local/go/bin/go

.PHONY: test
test:
	go test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out
