GO=go

.PHONY: test
test:
	$(GO) test . -v -race -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out
	@if [ -n "$$SUDO_UID" ] && [ -n "$$SUDO_GID" ]; then chown $$SUDO_UID:$$SUDO_GID cover.out; fi