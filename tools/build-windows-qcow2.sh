#!/usr/bin/env bash
#
# docs/MEMO-setup-windows-driver-isos.md に記載された手順を実行し、
# Windows Server インストール用 ISO から、virtio-win ドライバと cloudbase-init を
# 組み込んだビルド用 VM を起動する（作成される disk は qcow2 形式）。
#
# 前提条件（カレントディレクトリ基準）:
#   - ./isos/ 配下に、以下のいずれかの Windows Server EVAL ISO が配置されていること
#     （ユーザーが自己責任で入手する）。
#       Win2k22: SERVER_EVAL_x64FRE_ja-jp.iso
#       Win2k25: 26100.32230.260111-0550.lt_release_svc_refresh_SERVER_EVAL_x64FRE_ja-jp.iso
#   - ./isos/ 配下に virtio-win ドライバ ISO の stable 版が配置されていること
#     （ユーザーが自己責任で入手する）。
#       https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso
#   - ./unattend/ 配下に、cloudbase-init インストーラ
#     (CloudbaseInitSetup_1_1_8_x64.msi) が配置されていること
#     （ユーザーが自己責任で入手する）。
#   - tools/autounattend.xml-temp (本スクリプトと同じディレクトリ) が
#     存在すること。これをテンプレートとして、Windows Server 2022/2025 に
#     応じてドライバパス(2k22/2k25)を書き換えた autounattend.xml を
#     ./unattend/ 配下に生成する。
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_XML="${SCRIPT_DIR}/autounattend.xml-temp"

WORKDIR="$(pwd)"
ISOS_DIR="${WORKDIR}/isos"
UNATTEND_DIR="${WORKDIR}/unattend"
ASSETS_ISO_UUID_PREFIX="$(uuidgen | cut -c1-4)"
ASSETS_ISO="${ISOS_DIR}/assets-${ASSETS_ISO_UUID_PREFIX}.iso"
VIRTIO_ISO="${ISOS_DIR}/virtio-win.iso"
AUTOUNATTEND="${UNATTEND_DIR}/autounattend.xml"
CLOUDBASE_MSI="${UNATTEND_DIR}/CloudbaseInitSetup_1_1_8_x64.msi"

WIN2K22_ISO="${ISOS_DIR}/SERVER_EVAL_x64FRE_ja-jp.iso"
WIN2K25_ISO="${ISOS_DIR}/26100.32230.260111-0550.lt_release_svc_refresh_SERVER_EVAL_x64FRE_ja-jp.iso"

for command_name in genisoimage virt-install uuidgen; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "${command_name} command not found" >&2
    exit 1
  fi
done

echo "1. 実行前の準備を確認する"

if [ -f "${WIN2K22_ISO}" ]; then
  VERSION="2022"
  OS_VARIANT="win2k22"
  DRIVER_TAG="2k22"
  INSTALL_ISO="${WIN2K22_ISO}"
elif [ -f "${WIN2K25_ISO}" ]; then
  VERSION="2025"
  OS_VARIANT="win2k25"
  DRIVER_TAG="2k25"
  INSTALL_ISO="${WIN2K25_ISO}"
else
  echo "Windows Server インストール用 ISO が ${ISOS_DIR} に見つかりません。" >&2
  echo "次のいずれかを自己責任で入手して配置してください。" >&2
  echo "  Win2k22: $(basename "${WIN2K22_ISO}")" >&2
  echo "  Win2k25: $(basename "${WIN2K25_ISO}")" >&2
  exit 1
fi
echo "  Windows Server ${VERSION} 用 ISO を使用します: ${INSTALL_ISO}"

if [ ! -f "${VIRTIO_ISO}" ]; then
  echo "${VIRTIO_ISO} が見つかりません。" >&2
  echo "https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso から自己責任で入手して配置してください。" >&2
  exit 1
fi

if [ ! -f "${TEMPLATE_XML}" ]; then
  echo "${TEMPLATE_XML} が見つかりません。" >&2
  exit 1
fi

if [ ! -f "${CLOUDBASE_MSI}" ]; then
  echo "${CLOUDBASE_MSI} が見つかりません。" >&2
  echo "https://cloudbase.it/downloads/CloudbaseInitSetup_Stable_x64.msi から自己責任で入手し、CloudbaseInitSetup_1_1_8_x64.msi という名前で配置してください。" >&2
  exit 1
fi

echo "2. autounattend.xml をテンプレートから生成する (ドライバパス: ${DRIVER_TAG})"
mkdir -p "${UNATTEND_DIR}"
sed "s/2k22/${DRIVER_TAG}/g" "${TEMPLATE_XML}" > "${AUTOUNATTEND}"

echo "3. assets.iso イメージを作成する"
genisoimage -output "${ASSETS_ISO}" -volid ASSETS -joliet -rock "${UNATTEND_DIR}"

echo "4. 実行開始"
VM_NAME="windows${VERSION}-build"
QCOW2_PATH="${WORKDIR}/${VM_NAME}-${ASSETS_ISO_UUID_PREFIX}.qcow2"

sudo virt-install \
    --name "${VM_NAME}" \
    --memory 8192 \
    --vcpus 4 \
    --os-variant "${OS_VARIANT}" \
    --boot uefi \
    --tpm backend.type=emulator,backend.version=2.0,model=tpm-crb \
    --disk path="${QCOW2_PATH}",bus=scsi,format=qcow2,size=40 \
    --controller type=scsi,model=virtio-scsi \
    --network network=default,model=virtio \
    --graphics spice \
    --video model.type=qxl \
    --cdrom "${INSTALL_ISO}" \
    --disk "${VIRTIO_ISO}",device=cdrom \
    --disk "${ASSETS_ISO}",device=cdrom \
    --noautoconsole

echo "完了: ${QCOW2_PATH} の作成を開始しました。"
echo "進行状況は 'virsh console ${VM_NAME}' や virt-viewer などで確認してください。"
