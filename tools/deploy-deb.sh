#!/bin/bash
#
# deploy-deb.sh - ビルド済み deb パッケージを各ホストへ転送してインストールするスクリプト
#
# 前提条件:
#   - tools/build-deb.sh で dist/ 配下に deb ファイルが生成済みであること
#   - 各ホストへ SSH 公開鍵認証でアクセスできること
#
# 使用方法:
#   bash tools/deploy-deb.sh
#   bash tools/deploy-deb.sh -u ubuntu    # SSH ユーザーを指定する場合
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# デプロイ先ホスト一覧
HOSTS=(
    192.168.1.200
    192.168.1.201
    192.168.1.202
    192.168.1.203
)

# SSH ユーザー (デフォルト: 現在のユーザー)
SSH_USER="${USER}"

# オプション解析
while getopts "u:" opt; do
    case "${opt}" in
        u) SSH_USER="${OPTARG}" ;;
        *) echo "使用方法: $0 [-u <SSHユーザー>]" >&2; exit 1 ;;
    esac
done

TAG=$(cat "${ROOT_DIR}/TAG")
ARCH="amd64"
DEB_FILE="${ROOT_DIR}/dist/marmot_v${TAG}_${ARCH}.deb"

echo "=== marmot v${TAG} デプロイ ==="
echo ""

# deb ファイルの存在確認
if [ ! -f "${DEB_FILE}" ]; then
    echo "エラー: ${DEB_FILE} が見つかりません。"
    echo "先に deb パッケージをビルドしてください:"
    echo "  bash tools/build-deb.sh"
    exit 1
fi

echo "デプロイ対象: ${DEB_FILE}"
echo "接続ユーザー: ${SSH_USER}"
echo ""

REMOTE_TMP="/tmp/$(basename "${DEB_FILE}")"
FAILED_HOSTS=()
read -r -d '' REMOTE_POST_DEPLOY_SCRIPT <<'EOS' || true
set -euo pipefail

ensure_service_if_exists() {
    local unit_name="$1"
    if systemctl list-unit-files | grep -q "^${unit_name}\\.service"; then
        systemctl enable "${unit_name}.service" || true
        systemctl start "${unit_name}.service" || true
    fi
}

ensure_network_if_defined() {
    local network_name="$1"
    if ! command -v virsh >/dev/null 2>&1; then
        return 0
    fi
    if ! virsh net-info "${network_name}" >/dev/null 2>&1; then
        return 0
    fi
    virsh net-start "${network_name}" >/dev/null 2>&1 || true
    virsh net-autostart "${network_name}" >/dev/null 2>&1 || true
}

ensure_service_if_exists libvirtd
ensure_service_if_exists openvswitch-switch
ensure_service_if_exists ovsdb-server
ensure_service_if_exists ovs-vswitchd
ensure_service_if_exists ovn-central
ensure_service_if_exists ovn-northd
ensure_service_if_exists ovn-controller
ensure_service_if_exists ovn-host

if command -v ovs-vsctl >/dev/null 2>&1; then
    ovs-vsctl --may-exist add-br ovsbr0 || true
fi

ensure_network_if_defined default
ensure_network_if_defined host-bridge

systemctl restart marmot.service || systemctl start marmot.service || true
EOS

for HOST in "${HOSTS[@]}"; do
    echo "---------- ${HOST} ----------"

    # deb ファイルを転送
    echo "  転送中: ${DEB_FILE} → ${SSH_USER}@${HOST}:${REMOTE_TMP}"
    if ! scp -o StrictHostKeyChecking=no "${DEB_FILE}" "${SSH_USER}@${HOST}:${REMOTE_TMP}"; then
        echo "  [エラー] 転送失敗: ${HOST}"
        FAILED_HOSTS+=("${HOST}")
        continue
    fi

    # dpkg -i でインストール (sudo を使用)
    echo "  インストール中: dpkg -i ${REMOTE_TMP}"
    if ! ssh -o StrictHostKeyChecking=no "${SSH_USER}@${HOST}" \
            "sudo dpkg -i ${REMOTE_TMP} && rm -f ${REMOTE_TMP} && sudo bash -s" <<<"${REMOTE_POST_DEPLOY_SCRIPT}"; then
        echo "  [エラー] インストール失敗: ${HOST}"
        FAILED_HOSTS+=("${HOST}")
        continue
    fi

    echo "  [完了] ${HOST}"
    echo ""
done

echo "=============================="
if [ ${#FAILED_HOSTS[@]} -eq 0 ]; then
    echo "全ホストへのデプロイが完了しました。"
else
    echo "以下のホストでエラーが発生しました:"
    for h in "${FAILED_HOSTS[@]}"; do
        echo "  - ${h}"
    done
    exit 1
fi
