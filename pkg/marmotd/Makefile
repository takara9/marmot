GO=/usr/local/go/bin/go
PROGRAM	= marmotd
all:	$(PROGRAM)

TAG := $(shell cat ../../TAG)
BINDIR = ../../marmot-v$(TAG)

$(PROGRAM): $(%.go)
	echo $(BINDIR)
	mkdir -p $(BINDIR)
	go build -o $(BINDIR)/$@ $^

.PHONY:	clean
clean:
	rm -fr $(BINDIR)

.PHONY:	install
install:
	rm -f /usr/local/bin/$(PROGRAM)
	install -m 0755 $(PROGRAM) /usr/local/bin

.PHONY: test
test:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out

.PHONY: test-volume
test-volume:
	$(GO) test -ginkgo.focus-file volume_test.go -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events

.PHONY: test-server
test-server:
	$(GO) test -ginkgo.focus-file server_test.go -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events

.PHONY: test-func
test-func:
	$(GO) test -ginkgo.focus-file marmotd-funcs_test.go -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events

.PHONY:	set-version
set-version:
	sed s/MARMOT_VERSION/${TAG}/ cmd/version._ > cmd/version.go

.PHONY:	download-image
download-image:
	mc cp myminio/os-image/ubuntu22.04.img testdata/ubuntu22.04.qcow2


.PHONY: test-network
test-network:
	$(GO) test -ginkgo.focus="VirtualPrivateNetworks" -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events
