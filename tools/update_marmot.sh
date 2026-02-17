#!/bin/bash -e

echo "Copying marmot package to current directory..."
VERSION=`cat marmot/TAG`
mv marmot/marmot-v${VERSION}.tgz .
tar xzf marmot-v${VERSION}.tgz
cd marmot-v${VERSION}

echo "Stopping marmot service..."
sudo systemctl stop marmot

echo "Installing marmot version ${VERSION}..."

BINDIR=.
DISTDIR=/usr/local/marmot
SERVER_EXE=marmotd
CLIENT_CMD=mactl
CLIENT_CONFIG=config_marmot
ADMIN_CMD=maadm

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

echo "marmot version ${VERSION} is installed."

echo "Starting marmot service..."
sudo systemctl daemon-reload
sudo systemctl start marmot

echo "Waiting for marmot service to restart..."
sleep 3

mactl version
