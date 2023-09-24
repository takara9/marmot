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
