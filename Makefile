PROGRAMS = vm-server vm-client hv-admin
all:	$(PROGRAMS)

MAKE = /usr/bin/make
CURDIR := $(shell pwd)
BINDIR = $(CURDIR)/bin
TAG := $(shell cat TAG)

$(PROGRAMS):
	mkdir -p $(BINDIR)
	cd cmd/$@ && $(MAKE)

setup:
	env GOFLAGS= go install golang.org/x/tools/cmd/goimports@latest
	env GOFLAGS= go install honnef.co/go/tools/cmd/staticcheck@latest

test:
	test -z "$$(gofmt -s -l . | tee /dev/stderr)"
	staticcheck ./...

.PHONY:	package
package: all
	@echo $(TAG)
	cp cmd/install.sh bin/install.sh
	cp cmd/vm-client/config_marmot bin/config_marmot
	cd $(BINDIR) && tar czvf marmot-v$(TAG).tgz *


.PHONY:	clean
clean:
	rm -fr $(BINDIR)

DISTDIR = /usr/local/marmot
SERVER_EXE = vm-server
CLIENT_CMD = mactl
ADMIN_CMD  = hv-admin

.PHONY:	install
install:
	rm -fr $(DISTDIR)
	mkdir $(DISTDIR)
	install -m 0755 $(BINDIR)/$(SERVER_EXE) $(DISTDIR)
	install -m 0644 $(BINDIR)/temp.xml $(DISTDIR)
	rm -f /etc/systemd/system/marmot.service
	install -m 0644 $(BINDIR)/marmot.service /lib/systemd/system
	rm -f /usr/local/bin/$(CLIENT_CMD)
	install -m 0755 $(BINDIR)/$(CLIENT_CMD) /usr/local/bin
	rm -f /usr/local/bin/$(ADMIN_CMD)
	install -m 0755 $(BINDIR)/$(ADMIN_CMD) /usr/local/bin
