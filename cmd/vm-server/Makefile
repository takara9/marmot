PROGRAM	= vm-server
DISTDIR = /usr/local/marmot
all:	$(PROGRAM)
BINDIR = ../../bin

$(PROGRAM): $(%.go)
	echo $(BINDIR)
	mkdir -p $(BINDIR)
	go build -o $(BINDIR)/$@ $^
	cp temp.xml $(BINDIR)/temp.xml
	cp marmot.service $(BINDIR)/marmot.service

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
