#!/bin/bash

BINDIR=.
DISTDIR=/usr/local/marmot
SERVER_EXE=vm-server
CLIENT_CMD=mactl
ADMIN_CMD=hv-admin

rm -fr ${DISTDIR}
mkdir ${DISTDIR}
install -m 0755 ${BINDIR}/${SERVER_EXE} ${DISTDIR}
install -m 0644 ${BINDIR}/temp.xml ${DISTDIR}
rm -f /etc/systemd/system/marmot.service
install -m 0644 ${BINDIR}/marmot.service /etc/systemd/system
rm -f /usr/local/bin/${CLIENT_CMD}
install -m 0755 ${BINDIR}/${CLIENT_CMD} /usr/local/bin
rm -f /usr/local/bin/${ADMIN_CMD}
install -m 0755 ${BINDIR}/${ADMIN_CMD} /usr/local/bin

systemctl enable marmot
systemctl start marmot
