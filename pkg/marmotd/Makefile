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

.PHONY:	set-version
set-version:
	sed s/MARMOT_VERSION/${TAG}/ cmd/version._ > cmd/version.go

.PHONY:	download-image
download-image:
	mc cp myminio/os-image/ubuntu22.04.img testdata/ubuntu22.04.qcow2

.PHONY: test
test:
	$(GO) test . -run TestMarmotd -v -race -count=1 -timeout 20m -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events

.PHONY: test-volume
test-volume:
	$(GO) test -ginkgo.focus-file volume_test.go -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events

.PHONY: test-server
test-server:
	$(GO) test -ginkgo.focus-file server_test.go -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events

.PHONY: test-vnet
test-vnet:
	$(GO) test -ginkgo.focus-file network_vnet_test.go -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events

.PHONY: test-ipnet
test-ipnet:
	$(GO) test -ginkgo.focus-file network_ipnet_test.go -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events


.PHONY: test-server-image
test-server-image:
	$(GO) test -ginkgo.focus-file server-image_test.go -timeout 60m -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events



.PHONY: test-image
test-image:
	$(GO) test -ginkgo.focus-file image_test.go -timeout 20m -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events


.PHONY: test-iso
test-iso:
	$(GO) test -ginkgo.focus="Cloud-InitISO作成テスト" -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events
