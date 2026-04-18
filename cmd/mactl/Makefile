GO ?= go
PROGRAM	= mactl
GINKGO = /root/go/bin/ginkgo

all:	$(PROGRAM)

TAG := $(shell cat ../../TAG)
BINDIR = ../../dist/marmot-v$(TAG)
TESTBINDIR = bin

$(PROGRAM): $(%.go)
	echo $(BINDIR)
	mkdir -p $(BINDIR)
	$(GO) build -o $(BINDIR)/$@ $^

.PHONY:	mactl-test
mactl-test: $(%.go)
	echo $(TESTBINDIR)
	mkdir -p $(TESTBINDIR)
	$(GO) build -o $(TESTBINDIR)/$@ $^

.PHONY:	clean
clean:
	rm -fr $(BINDIR)
	rm -fr $(TESTBINDIR)

.PHONY:	install
install:
	rm -f /usr/local/bin/$(PROGRAM)
	install -m 0755 $(PROGRAM) /usr/local/bin

.PHONY: test0
test0: mactl-test
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus-file mactl_test.go -coverprofile cover.out

.PHONY:test1 
test1:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus-file mactl-vm-deploy-1_test.go -timeout=20m -coverprofile cover.out

.PHONY:test2
test2:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus-file mactl-vm-deploy-2_test.go -timeout=20m -coverprofile cover.out

.PHONY:test3
test3:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus-file mactl-vm-deploy-3_test.go -timeout=20m -coverprofile cover.out
