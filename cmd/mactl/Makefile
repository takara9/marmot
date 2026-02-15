GO ?= go
PROGRAM	= mactl
GINKGO = /root/go/bin/ginkgo

all:	$(PROGRAM)

TAG := $(shell cat ../../TAG)
BINDIR = ../../marmot-v$(TAG)
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

.PHONY: test
test: mactl-test
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out
	#$(GINKGO) -v --race --focus-file mactl_test.go

.PHONY: test2
test2: mactl-test
	$(GINKGO) -v --focus-file mactl-new_test.go
