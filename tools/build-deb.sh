#!/bin/bash
#
# build-deb.sh - marmot dpkg パッケージビルドスクリプト
#
# 生成するパッケージ: marmot_<VERSION>_amd64.deb
#
# 前提条件:
#   - 以下のバイナリが marmot-v<VERSION>/ ディレクトリに存在すること
#       marmotd, mactl
#   - dpkg-deb コマンドが利用可能であること
#
# 使用方法:
#   cd /path/to/marmot
#   make all          # 各バイナリをビルド
#   bash tools/build-deb.sh
#
# インストール方法:
#   sudo dpkg -i dist/marmot_<VERSION>_amd64.deb
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

TAG=$(cat "${ROOT_DIR}/TAG")
ARCH="amd64"
PACKAGE_NAME="marmot"

BINDIR="${ROOT_DIR}/dist/marmot-v${TAG}"
DIST_DIR="${ROOT_DIR}/dist"
PKG_DIR="${DIST_DIR}/${PACKAGE_NAME}_v${TAG}_${ARCH}"
DEB_FILE="${PACKAGE_NAME}_v${TAG}_${ARCH}.deb"

echo "=== marmot v${TAG} dpkgパッケージビルド ==="
echo ""

# バイナリの存在確認
for bin in marmotd mactl maadm; do
    if [ ! -f "${BINDIR}/${bin}" ]; then
        echo "エラー: ${BINDIR}/${bin} が見つかりません。"
        echo "先に各コマンドをビルドしてください:"
        echo "  (cd cmd/marmotd && make)"
        echo "  (cd cmd/mactl   && make)"
        echo "  (cd cmd/maadm   && make)"
        exit 1
    fi
done

# dpkg-deb コマンドの確認
if ! command -v dpkg-deb &>/dev/null; then
    echo "エラー: dpkg-deb コマンドが見つかりません。"
    echo "  sudo apt install dpkg-dev"
    exit 1
fi

# 出力ディレクトリの初期化
rm -rf "${PKG_DIR}"
mkdir -p "${PKG_DIR}/DEBIAN"
mkdir -p "${PKG_DIR}/usr/local/marmot"
mkdir -p "${PKG_DIR}/usr/local/marmot/gateway-playbooks"
mkdir -p "${PKG_DIR}/usr/local/bin"
mkdir -p "${PKG_DIR}/lib/systemd/system"
mkdir -p "${PKG_DIR}/etc/marmot"
mkdir -p "${PKG_DIR}/etc/marmot/keys"
mkdir -p "${PKG_DIR}/var/lib/marmot/images"
mkdir -p "${PKG_DIR}/var/lib/marmot/ansible-playbooks/templates"
mkdir -p "${PKG_DIR}/var/lib/marmot/isos"
mkdir -p "${PKG_DIR}/var/lib/marmot/jobs"
mkdir -p "${PKG_DIR}/var/lib/marmot/volumes"

echo "バイナリをコピー中..."
install -m 0755 "${BINDIR}/marmotd" "${PKG_DIR}/usr/local/marmot/marmotd"
install -m 0755 "${BINDIR}/mactl"   "${PKG_DIR}/usr/local/bin/mactl"
install -m 0755 "${BINDIR}/maadm"   "${PKG_DIR}/usr/local/bin/maadm"
install -m 0644 "${ROOT_DIR}/pkg/controller/gateway-playbooks/gateway-iptables.yaml.tmpl" \
    "${PKG_DIR}/usr/local/marmot/gateway-playbooks/gateway-iptables.yaml.tmpl"
install -m 0644 "${ROOT_DIR}/pkg/controller/gateway-playbooks/vpn-gateway-openvpn.yaml.tmpl" \
    "${PKG_DIR}/usr/local/marmot/gateway-playbooks/vpn-gateway-openvpn.yaml.tmpl"

echo "systemd サービスファイルをコピー中..."
install -m 0644 "${ROOT_DIR}/cmd/marmotd/marmot.service" \
    "${PKG_DIR}/lib/systemd/system/marmot.service"

install -m 0644 "${ROOT_DIR}/cmd/marmotd/marmotd.json" \
    "${PKG_DIR}/etc/marmot/marmotd.json"

echo "設定ファイルのサンプルをコピー中..."
install -m 0644 "${ROOT_DIR}/cmd/mactl/.marmot.example" \
    "${PKG_DIR}/etc/marmot/.marmot.example"

# インストール済みサイズを計算 (KB単位)
INSTALLED_SIZE=$(du -sk "${PKG_DIR}" | cut -f1)

echo "DEBIAN/control を生成中..."
cat > "${PKG_DIR}/DEBIAN/control" <<EOF
Package: ${PACKAGE_NAME}
Version: ${TAG}
Architecture: ${ARCH}
Maintainer: Marmot Team
Installed-Size: ${INSTALLED_SIZE}
Depends: libvirt-daemon-system,
 libvirt-daemon,
 libvirt-clients,
 libvirt-daemon-driver-lxc,
 libvirt-dev,
 libguestfs-tools,
 qemu-system-x86,
 openvswitch-switch,
 openvswitch-common,
 ovn-central,
 ovn-host,
 bridge-utils,
 lxcfs,
 kpartx,
 genisoimage,
 nfs-common,
 lvm2,
 etcd-server,
 open-iscsi,
 targetcli-fb,
 ansible-core
Section: admin
Priority: optional
Description: marmot - VM クラスター管理サービス
 marmot はVMクラスターを管理するサービスです。
 サーバーデーモン (marmotd)、クライアントCLI (mactl)、
 を含みます。
EOF

echo "DEBIAN/conffiles を生成中..."
cat > "${PKG_DIR}/DEBIAN/conffiles" <<'CONFFILES'
/etc/marmot/marmotd.json
/etc/marmot/.marmot.example
CONFFILES

echo "DEBIAN/postinst を生成中..."
install -m 0755 "${ROOT_DIR}/tools/deb/postinst" "${PKG_DIR}/DEBIAN/postinst"

echo "DEBIAN/prerm を生成中..."
install -m 0755 "${ROOT_DIR}/tools/deb/prerm" "${PKG_DIR}/DEBIAN/prerm"

echo "DEBIAN/postrm を生成中..."
install -m 0755 "${ROOT_DIR}/tools/deb/postrm" "${PKG_DIR}/DEBIAN/postrm"

echo "dpkgパッケージをビルド中..."
dpkg-deb --build --root-owner-group "${PKG_DIR}" "${DIST_DIR}/${DEB_FILE}"

echo ""
echo "=== ビルド完了 ==="
echo "パッケージ: ${DEB_FILE}"
echo ""
echo "インストール (_apt 警告を避ける手順):"
echo "  sudo install -m 0644 ./${DEB_FILE} /tmp/"
echo "  sudo apt install /tmp/${DEB_FILE}"
echo "  sudo rm -f /tmp/${DEB_FILE}"
echo ""
echo "アンインストール:"
echo "  sudo apt remove ${PACKAGE_NAME}"
echo ""
