PROGRAM	= vm-server
DISTDIR = /usr/local/marmot
all:	$(PROGRAM)

TAG := $(shell cat ../../TAG)
BINDIR = ../../marmot-v$(TAG)

$(PROGRAM): $(%.go)
	echo $(BINDIR)
	mkdir -p $(BINDIR)
	go build -o $(BINDIR)/$@ $^
	cp temp.xml $(BINDIR)/temp.xml
	cp marmot.service $(BINDIR)/marmot.service

.PHONY:	clean
clean:
	rm -f $(PROGRAM)

.PHONY: test
test:
	$(GO) test -v -race . -ginkgo.v -ginkgo.fail-fast -ginkgo.show-node-events -coverprofile cover.out

.PHONY:	install
install:
	rm -fr $(DISTDIR)
	mkdir $(DISTDIR)
	install -m 0755 $(PROGRAM) $(DISTDIR)
	install -m 0644 temp.xml $(DISTDIR)
	rm -f /etc/systemd/system/marmot.service
	install -m 0644 marmot.service /etc/systemd/system
