#!/bin/bash
#
# build-deb.sh - marmot dpkg パッケージビルドスクリプト
#
# 生成するパッケージ: marmot_<VERSION>_amd64.deb
#
# 前提条件:
#   - 以下のバイナリが marmot-v<VERSION>/ ディレクトリに存在すること
#       marmotd, mactl, maadm
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

BINDIR="${ROOT_DIR}/marmot-v${TAG}"
DIST_DIR="${ROOT_DIR}/dist"
PKG_DIR="${DIST_DIR}/${PACKAGE_NAME}_${TAG}_${ARCH}"
DEB_FILE="${DIST_DIR}/${PACKAGE_NAME}_${TAG}_${ARCH}.deb"

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
mkdir -p "${PKG_DIR}/usr/local/bin"
mkdir -p "${PKG_DIR}/lib/systemd/system"
mkdir -p "${PKG_DIR}/etc/marmot"
mkdir -p "${PKG_DIR}/var/lib/marmot/images"
mkdir -p "${PKG_DIR}/var/lib/marmot/isos"
mkdir -p "${PKG_DIR}/var/lib/marmot/jobs"
mkdir -p "${PKG_DIR}/var/lib/marmot/volumes"

echo "バイナリをコピー中..."
install -m 0755 "${BINDIR}/marmotd" "${PKG_DIR}/usr/local/marmot/marmotd"
install -m 0755 "${BINDIR}/mactl"   "${PKG_DIR}/usr/local/bin/mactl"
install -m 0755 "${BINDIR}/maadm"   "${PKG_DIR}/usr/local/bin/maadm"

echo "systemd サービスファイルをコピー中..."
# XXXHOSTXXXX はインストール後の postinst スクリプトで実際のホスト名に置換される
install -m 0644 "${ROOT_DIR}/cmd/marmotd/marmot.service" \
    "${PKG_DIR}/lib/systemd/system/marmot.service"

echo "設定ファイルのサンプルをコピー中..."
install -m 0644 "${ROOT_DIR}/cmd/mactl/config_marmot" \
    "${PKG_DIR}/etc/marmot/config_marmot.example"

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
 bridge-utils,
 lxcfs,
 kpartx,
 genisoimage,
 nfs-common,
 lvm2,
 etcd
Section: admin
Priority: optional
Description: marmot - VM クラスター管理サービス
 marmot はVMクラスターを管理するサービスです。
 サーバーデーモン (marmotd)、クライアントCLI (mactl)、
 管理者CLI (maadm) を含みます。
EOF

echo "DEBIAN/postinst を生成中..."
cat > "${PKG_DIR}/DEBIAN/postinst" <<'POSTINST'
#!/bin/bash
set -e

SERVICE_FILE="/lib/systemd/system/marmot.service"
NODE_HOSTNAME="$(hostname)"

# サービスファイル内のホスト名プレースホルダーを置換
if grep -q "XXXHOSTXXXX" "${SERVICE_FILE}" 2>/dev/null; then
    sed -i "s/XXXHOSTXXXX/${NODE_HOSTNAME}/g" "${SERVICE_FILE}"
fi

# LXC を有効化するために libvirtd を停止・無効化して lxcfs を起動する
if systemctl is-active --quiet libvirtd.service 2>/dev/null; then
    systemctl stop libvirtd.service
fi
if systemctl is-enabled --quiet libvirtd.service 2>/dev/null; then
    systemctl disable libvirtd.service
fi
systemctl enable lxcfs.service
systemctl start lxcfs.service

systemctl daemon-reload
systemctl enable marmot.service

echo ""
echo "============================================================"
echo " marmot のインストールが完了しました"
echo "============================================================"
echo ""
echo "【次のステップ1】LVM ボリュームグループの設定 (環境に応じて実施)"
echo ""
echo "  ディスクを確認する:"
echo "    lsblk"
echo ""
echo "  Physical Volume を作成する (例: /dev/vdc, /dev/vdd を使用):"
echo "    pvcreate /dev/vdc"
echo "    pvcreate /dev/vdd"
echo ""
echo "  Volume Group を作成する:"
echo "    vgcreate vg1 /dev/vdc"
echo "    vgcreate vg2 /dev/vdd"
echo ""
echo "  Logical Volume を作成する (例: テンプレート用 16GB):"
echo "    lvcreate --name lv01 --size 16GB vg1"
echo ""
echo "  確認:"
echo "    vgs"
echo "    lvs"
echo ""
echo "------------------------------------------------------------"
echo ""
echo "【次のステップ2】ネットワーク (OVS ブリッジ) の設定 (環境に応じて実施)"
echo ""
echo "  (1) OVS ブリッジを作成して物理ポートを接続し、VLANトランクを設定する:"
echo "    ovs-vsctl add-br ovsbr0"
echo "    ovs-vsctl add-port ovsbr0 <物理NIC名>          # 例: enp4s0f0"
echo "    ovs-vsctl set port <物理NIC名> trunk=1001,1002"
echo "    ovs-vsctl show"
echo ""
echo "  (2) netplan でブリッジ (br0) を設定し、IPアドレスを割り当てる:"
echo "    vi /etc/netplan/00-nic.yaml"
echo "    netplan apply"
echo ""
echo "  (3) libvirt 仮想ネットワークを定義・有効化する:"
echo "    virsh net-define ovs-network.xml"
echo "    virsh net-start ovs-network"
echo "    virsh net-autostart ovs-network"
echo "    virsh net-define host-bridge.xml"
echo "    virsh net-start host-bridge"
echo "    virsh net-autostart host-bridge"
echo "    virsh net-list"
echo ""
echo "  詳細は以下のドキュメントを参照してください:"
echo "    docs/HOWTO-setup-vm-runner.md"
echo "    docs/network-setup.md"
echo ""
echo "------------------------------------------------------------"
echo ""
echo "【次のステップ3】marmot サービスの起動"
echo ""
echo "  クライアント設定を配置する (サンプルをコピーして編集):"
echo "    cp /etc/marmot/config_marmot.example ~/.config_marmot"
echo "    vi ~/.config_marmot"
echo ""
echo "  サービスを開始する:"
echo "    systemctl start marmot"
echo "    systemctl status marmot"
echo ""
echo "============================================================"

exit 0
POSTINST
chmod 0755 "${PKG_DIR}/DEBIAN/postinst"

echo "DEBIAN/prerm を生成中..."
cat > "${PKG_DIR}/DEBIAN/prerm" <<'PRERM'
#!/bin/bash
set -e

if systemctl is-active --quiet marmot.service 2>/dev/null; then
    echo "marmot サービスを停止中..."
    systemctl stop marmot.service
fi

if systemctl is-enabled --quiet marmot.service 2>/dev/null; then
    systemctl disable marmot.service
fi

exit 0
PRERM
chmod 0755 "${PKG_DIR}/DEBIAN/prerm"

echo "DEBIAN/postrm を生成中..."
cat > "${PKG_DIR}/DEBIAN/postrm" <<'POSTRM'
#!/bin/bash
set -e

systemctl daemon-reload || true

if [ "$1" = "purge" ]; then
    rm -rf /usr/local/marmot
    rm -rf /etc/marmot
fi

exit 0
POSTRM
chmod 0755 "${PKG_DIR}/DEBIAN/postrm"

echo "dpkgパッケージをビルド中..."
dpkg-deb --build --root-owner-group "${PKG_DIR}" "${DEB_FILE}"

echo ""
echo "=== ビルド完了 ==="
echo "パッケージ: ${DEB_FILE}"
echo ""
echo "インストール:"
echo "  sudo dpkg -i ${DEB_FILE}"
echo ""
echo "アンインストール:"
echo "  sudo dpkg -r ${PACKAGE_NAME}"
echo ""
