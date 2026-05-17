#!/bin/bash

BINDIR=.
DISTDIR=/usr/local/marmot
SERVER_EXE=marmotd
CLIENT_CMD=mactl
CLIENT_CONFIG=config_marmot
ADMIN_CMD=maadm

rm -fr ${DISTDIR}
mkdir ${DISTDIR}
install -m 0755 ${BINDIR}/${SERVER_EXE} ${DISTDIR}

rm -f /etc/systemd/system/marmot.service
HOSTNAME=`hostname`
sed -i s%XXXHOSTXXXX%${HOSTNAME}% ${BINDIR}/marmot.service
install -m 0644 ${BINDIR}/marmot.service /etc/systemd/system

rm -f /usr/local/bin/${CLIENT_CMD}
install -m 0755 ${BINDIR}/${CLIENT_CMD} /usr/local/bin
rm -f /usr/local/bin/${ADMIN_CMD}
install -m 0755 ${BINDIR}/${ADMIN_CMD} /usr/local/bin
install -m 0644 ${BINDIR}/${CLIENT_CONFIG} ${HOME}/.${CLIENT_CONFIG}

# Open vSwitch を有効化して起動する
if systemctl list-unit-files | grep -q '^openvswitch-switch\.service'; then
	systemctl enable openvswitch-switch.service
	systemctl start openvswitch-switch.service
elif systemctl list-unit-files | grep -q '^ovs-vswitchd\.service'; then
	# ディストリ差異向けのフォールバック
	systemctl enable ovsdb-server.service || true
	systemctl start ovsdb-server.service || true
	systemctl enable ovs-vswitchd.service
	systemctl start ovs-vswitchd.service
fi

systemctl daemon-reload
systemctl enable marmot
systemctl start marmot
