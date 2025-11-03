GO=/usr/local/go/bin/go
PROGRAM	= maadm
all:	$(PROGRAM)

TAG := $(shell cat ../../TAG)
BINDIR = ../../marmot-v$(TAG)
TESTBINDIR = bin

$(PROGRAM): $(%.go)
	echo $(BINDIR)
	mkdir -p $(BINDIR)
	$(GO) build -o $(BINDIR)/$@ $^

maadm-test: $(%.go)
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
test:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out
