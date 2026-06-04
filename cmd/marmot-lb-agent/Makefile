GO ?= go
PROGRAM = marmot-lb-agent

.PHONY: all $(PROGRAM)
all: $(PROGRAM)

TAG := $(shell cat ../../TAG)
BINDIR = ../../dist/marmot-v$(TAG)

$(PROGRAM): $(%.go)
	echo $(BINDIR)
	mkdir -p $(BINDIR)
	$(GO) build -o $(BINDIR)/$@ $^

.PHONY: clean
clean:
	rm -fr $(BINDIR)

.PHONY: install
install:
	rm -f /usr/local/bin/$(PROGRAM)
	install -m 0755 $(PROGRAM) /usr/local/bin