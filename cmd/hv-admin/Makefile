PROGRAM	= hv-admin
all:	$(PROGRAM)

.PHONY:	deps
deps:
		go mod tidy

$(PROGRAM):	$(%.go)
		go build -o $@ $^

.PHONY:	clean
clean:
		rm -f $(PROGRAM)

.PHONY:	install
install:
		rm -f /usr/local/bin/$(PROGRAM)
		install -m 0755 $(PROGRAM) /usr/local/bin
		