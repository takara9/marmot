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
	$(GINKGO) -v --race --focus-file mamortd-main_test.go

#$(GINKGO) -v --race --focus-file marmotd_test.go 

.PHONY:	set-version
set-version:
	sed s/MARMOT_VERSION/${TAG}/ cmd/version._ > cmd/version.go
