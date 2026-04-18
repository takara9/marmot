GO=/usr/local/go/bin/go

.PHONY: test
test:
	$(GO) test . -ginkgo.focus="Lvm" -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out
	chown ubuntu:ubuntu cover.out
