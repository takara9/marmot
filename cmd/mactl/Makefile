GO ?= /usr/local/go/bin/go
PROGRAM	= mactl
GINKGO = /root/go/bin/ginkgo
export CEPH_IPADDR=10.1.3.11
export CEPH_POOL_KEY=AQCAD1dq4k4jARAAq7ckh5t6aouhEGokyef0Fg==

.PHONY: all $(PROGRAM)

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
	$(GO) test -v . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus-file mactl-vm-deploy-1_test.go -timeout=20m -coverprofile cover.out

.PHONY:test2
test2:
	$(GO) test -v . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus-file mactl-vm-deploy-2_test.go -timeout=20m -coverprofile cover.out

.PHONY:test3
test3:
	$(GO) test -v . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus-file mactl-vm-deploy-3_test.go -timeout=20m -coverprofile cover.out

.PHONY: test-unit
test-unit:
	$(GO) test ./cmd -v -ginkgo.v -timeout=5m

.PHONY:test-ceph
test-ceph:
	echo $$CEPH_IPADDR
	echo $$CEPH_POOL_KEY
	$(GO) test . -ginkgo.focus="Ceph volume backed VM lifecycle" -count=1
	#$(GO) test -v . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -ginkgo.focus-file mactl-ceph-lifecycle_test.go -timeout=10m -coverprofile cover.out

