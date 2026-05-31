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

# OVN/OVS サービスを有効化して起動する（存在するユニットのみ対象）
enable_and_start_if_exists() {
	unit_name="$1"
	if systemctl list-unit-files | grep -q "^${unit_name}\\.service"; then
		systemctl enable "${unit_name}.service" || true
		systemctl start "${unit_name}.service" || true
	fi
}

# OVS 基盤
enable_and_start_if_exists openvswitch-switch
enable_and_start_if_exists ovsdb-server
enable_and_start_if_exists ovs-vswitchd

# OVN 制御プレーン/データプレーン
enable_and_start_if_exists ovn-central
enable_and_start_if_exists ovn-northd
enable_and_start_if_exists ovn-controller
enable_and_start_if_exists ovn-host

systemctl daemon-reload
systemctl enable marmot
systemctl start marmot
