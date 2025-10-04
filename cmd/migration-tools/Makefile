PROGRAM	= migtool
all:	$(PROGRAM)

TAG := $(shell cat ../../TAG)
BINDIR = ../../marmot-v$(TAG)

$(PROGRAM): $(%.go)
	echo $(BINDIR)
	mkdir -p $(BINDIR)
	go build -o $(BINDIR)/$@ $^

.PHONY:	clean
clean:
	rm -f $(PROGRAM)

.PHONY:	install
install:
	rm -f /usr/local/bin/$(PROGRAM)
	install -m 0755 $(PROGRAM) /usr/local/bin

.PHONY: test
test:
	go test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out
	#$(GINKGO) -v --race --focus-file migtool_test.go
