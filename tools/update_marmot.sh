#!/bin/bash -e

if [ "$(basename "$PWD")" != "marmot" ]; then
	echo "Error: run this script from the marmot directory. Current directory: $PWD" >&2
	exit 1
fi

if [ "$EUID" -ne 0 ]; then
	echo "Error: this script must be run as root." >&2
	exit 1
fi

echo "Copying marmot package to current directory..."
VERSION=`cat TAG`
cd dist/marmot-v${VERSION}

echo "Stopping marmot service..."
sudo systemctl stop marmot

echo "Installing marmot version ${VERSION}..."

BINDIR=$(pwd)
DISTDIR=/usr/local/marmot
SERVER_EXE=marmotd
CLIENT_CMD=mactl
CLIENT_CONFIG=config_marmot
ADMIN_CMD=maadm

install -m 0755 ${BINDIR}/${SERVER_EXE} ${DISTDIR}

rm -f /etc/systemd/system/marmot.service
install -m 0644 ${BINDIR}/marmot.service /etc/systemd/system

rm -f /usr/local/bin/${CLIENT_CMD}
install -m 0755 ${BINDIR}/${CLIENT_CMD} /usr/local/bin

rm -f /usr/local/bin/${ADMIN_CMD}
install -m 0755 ${BINDIR}/${ADMIN_CMD} /usr/local/bin

rm -f /etc/marmot/marmotd.json
install -m 0644 ${BINDIR}/marmotd.json /etc/marmot

rm -f /etc/marmot/.marmot.example
install -m 0644 ${BINDIR}/.marmot.example /etc/marmot

echo "marmot version ${VERSION} is installed."

echo "Starting marmot service..."
sudo systemctl daemon-reload
sudo systemctl start marmot

echo "Waiting for marmot service to restart..."
sleep 3


mactl version
