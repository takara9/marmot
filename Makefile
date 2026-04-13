PROGRAMS = mactl marmotd maadm
.PHONY: all $(PROGRAMS)
all:	$(PROGRAMS)

MAKE = /usr/bin/make
CURDIR := $(shell pwd)
TAG := $(shell cat TAG)
BINDIR = dist/marmot-v$(TAG)

$(PROGRAMS):
	mkdir -p $(BINDIR)
	cd cmd/$@ && $(MAKE)

generate:
	oapi-codegen -config api/config-v1.yaml api/marmot-api-v1.yaml
	npx @redocly/cli build-docs api/marmot-api-v1.yaml -o api/marmot-api-v1.html
	go mod tidy

setup:
	env GOFLAGS= go install golang.org/x/tools/cmd/goimports@latest
	env GOFLAGS= go install honnef.co/go/tools/cmd/staticcheck@latest
	env GOFLAGS= go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

test: setup
	test -z "$$(gofmt -s -l . | tee /dev/stderr)"
	staticcheck ./...

.PHONY:	package
package: clean all setup
	@echo $(TAG)
	mkdir -p $(BINDIR)
	cp TAG pkg/marmotd/version.txt
	cp TAG cmd/mactl/cmd/version.txt
	cp TAG cmd/maadm/cmd/version.txt
	cp cmd/install.sh $(BINDIR)/install.sh
	cp cmd/mactl/.marmot.example $(BINDIR)/.marmot.example
	cp cmd/marmotd/marmot.service $(BINDIR)/marmot.service
	cp cmd/marmotd/marmotd.json $(BINDIR)/marmotd.json
	tar -C $(BINDIR) -czvf dist/marmot-v$(TAG).tgz .
	tools/build-deb.sh

.PHONY:	clean
clean:
	rm -fr dist

DISTDIR = /usr/local/marmot
SERVER_EXE = marmotd
CLIENT_CMD = mactl
ADMIN_CMD  = maadm

.PHONY:	install
install:
	rm -fr $(DISTDIR)
	mkdir $(DISTDIR)
	install -m 0755 $(BINDIR)/$(SERVER_EXE) $(DISTDIR)
	rm -f /etc/systemd/system/marmot.service
	install -m 0644 $(BINDIR)/marmot.service /lib/systemd/system
	rm -f /usr/local/bin/$(CLIENT_CMD)
	install -m 0755 $(BINDIR)/$(CLIENT_CMD) /usr/local/bin
	rm -f /usr/local/bin/$(ADMIN_CMD)
	install -m 0755 $(BINDIR)/$(ADMIN_CMD) /usr/local/bin
