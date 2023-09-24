PROGRAM	= vm-server
DISTDIR = /usr/local/marmot
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
		rm -fr $(DISTDIR)
		mkdir $(DISTDIR)
		install -m 0755 $(PROGRAM) $(DISTDIR)
		install -m 0644 temp.xml $(DISTDIR)
		rm -f /etc/systemd/system/marmot.service
		install -m 0644 marmot.service /etc/systemd/system

