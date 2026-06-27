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

# TLS 証明書の生成
CERT_DIR="/etc/marmot/certs"
TLS_CERT_FILE="${CERT_DIR}/server.crt"
TLS_KEY_FILE="${CERT_DIR}/server.key"

mkdir -p "${CERT_DIR}"
chmod 700 "${CERT_DIR}"

# 証明書とキーが存在しない場合、自己署名証明書を生成
if [ ! -f "${TLS_CERT_FILE}" ] || [ ! -f "${TLS_KEY_FILE}" ]; then
	rm -f "${TLS_CERT_FILE}" "${TLS_KEY_FILE}"
	
	# ホスト名を取得
	HOSTNAME=$(hostname)
	
	# 秘密鍵を生成
	openssl genrsa -out "${TLS_KEY_FILE}" 2048 >/dev/null 2>&1
	
	# 自己署名証明書を生成（365日有効）
	openssl req -new -x509 -key "${TLS_KEY_FILE}" -out "${TLS_CERT_FILE}" \
		-days 365 -subj "/CN=${HOSTNAME}/O=Marmot/C=JP" >/dev/null 2>&1
fi

chmod 600 "${TLS_KEY_FILE}"
chmod 644 "${TLS_CERT_FILE}"

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
