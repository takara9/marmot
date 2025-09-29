PROGRAMS = vm-server vm-client hv-admin mactl2 marmotd
all:	$(PROGRAMS)

MAKE = /usr/bin/make
CURDIR := $(shell pwd)
TAG := $(shell cat TAG)
BINDIR = $(CURDIR)/marmot-v$(TAG)

$(PROGRAMS):
	oapi-codegen -config api/config-v1.yaml api/marmot-api-v1.yaml
	npx @redocly/cli build-docs api/marmot-api-v1.yaml -o api/marmot-api-v1.html
	mkdir -p $(BINDIR)
	cd cmd/$@ && $(MAKE)

setup:
	cp TAG pkg/marmotd/version.txt
	cp TAG cmd/mactl2/version.txt
	env GOFLAGS= go install golang.org/x/tools/cmd/goimports@latest
	env GOFLAGS= go install honnef.co/go/tools/cmd/staticcheck@latest
	env GOFLAGS= go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

test:
	test -z "$$(gofmt -s -l . | tee /dev/stderr)"
	staticcheck ./...

.PHONY:	package
package: clean all
	@echo $(TAG)
	cp cmd/install.sh $(BINDIR)/install.sh
	cp cmd/vm-client/config_marmot $(BINDIR)/config_marmot
	tar czvf marmot-v$(TAG).tgz marmot-v$(TAG)

.PHONY:	clean
clean:
	rm -fr $(BINDIR)
	rm -f marmot-v$(TAG).tgz 

DISTDIR = /usr/local/marmot
#SERVER_EXE = vm-server
#CLIENT_CMD = mactl
SERVER_EXE = marmotd
CLIENT_CMD = mactl2
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
