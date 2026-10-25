#!/usr/bin/env bash
#
# docs/MEMO-setup-windows-driver-isos.md に記載された手順を実行し、
# Windows Server インストール用 ISO から、virtio-win ドライバと cloudbase-init を
# 組み込んだビルド用 VM を起動する（作成される disk は qcow2 形式）。
#
# 使い方: build-windows-qcow2.sh [オプション]   (詳細は -h/--help)
#
# 前提条件（作業ディレクトリ配下の build_qcow2/ 基準。既定はカレントディレクトリ、--workdir で変更可）:
#   - ./build_qcow2/isos/ 配下に、以下のいずれかの Windows Server EVAL ISO が配置されていること
#     （ファイル名は環境変数 WIN2K22_ISO / WIN2K25_ISO でパスごと上書き可能。
#       両方存在する場合は --version の指定が必須）
#     （ユーザーが自己責任で入手する）。
#       Win2k22: SERVER_EVAL_x64FRE_ja-jp.iso
#       Win2k25: 26100.32230.260111-0550.lt_release_svc_refresh_SERVER_EVAL_x64FRE_ja-jp.iso
#   - ./build_qcow2/isos/ 配下に virtio-win ドライバ ISO の stable 版が配置されていること
#     （ユーザーが自己責任で入手する）。
#       https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/stable-virtio/virtio-win.iso
#   - ./build_qcow2/unattend/ 配下に、cloudbase-init インストーラ
#     (CloudbaseInitSetup_1_1_8_x64.msi) が配置されていること
#     （ユーザーが自己責任で入手する）。
#   - tools/autounattend.xml-temp (本スクリプトと同じディレクトリ) が
#     存在すること。これをテンプレートとして、Windows Server 2022/2025 に
#     応じてドライバパス(2k22/2k25)を書き換えた autounattend.xml を
#     ./build_qcow2/unattend/ 配下に生成する。生成した qcow2 は ./build_qcow2/<VM名>.qcow2 に出力される
#     （実行時に ./build_qcow2/ 配下の既存の qcow2 は削除される）。
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEMPLATE_XML="${SCRIPT_DIR}/autounattend.xml-temp"

usage() {
  cat <<EOF
使い方: $(basename "$0") [オプション]

Windows Server インストール用 ISO から、virtio-win ドライバと cloudbase-init を
組み込んだビルド用 VM を起動します（disk は qcow2 形式）。

オプション:
  -h, --help             このヘルプを表示する
      --version VER      Windows Server のバージョン (2022 | 2025)
                         ISO が 1 つだけ存在する場合は省略可。両方存在する場合は必須
      --workdir DIR      作業ディレクトリ (既定: カレントディレクトリ)
                         DIR/build_qcow2/ 配下の isos/, unattend/ を入力に使い、qcow2 もここへ出力する
      --name NAME        VM 名 (既定: windows<VER>-build)
      --memory MB        メモリ容量 MiB (既定: 8192)
      --vcpus N          vCPU 数 (既定: 4)
      --disk-size GB     ディスク容量 GiB (既定: 40)
      --network NAME     libvirt ネットワーク名 (既定: default)
      --force            同名 VM が存在する場合、停止・削除（NVRAM 含む）して作り直す
                         （build_qcow2/ 配下の qcow2 は、同名 VM の有無に関わらず実行時に削除される）

環境変数:
  WIN2K22_ISO            Windows Server 2022 ISO のパス
  WIN2K25_ISO            Windows Server 2025 ISO のパス
EOF
}

die() {
  echo "$*" >&2
  exit 1
}

require_positive_int() {
  case "$2" in
    ''|*[!0-9]*|0) die "$1 には 1 以上の整数を指定してください: $2" ;;
  esac
}

VERSION=""
WORKDIR="$(pwd)"
VM_NAME=""
MEMORY_MB=8192
VCPUS=4
DISK_SIZE_GB=40
NETWORK="default"
FORCE=false

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --force) FORCE=true; shift ;;
    --version|--workdir|--name|--memory|--vcpus|--disk-size|--network)
      [ $# -ge 2 ] || die "$1 には値が必要です。"
      case "$1" in
        --version) VERSION="$2" ;;
        --workdir) WORKDIR="$2" ;;
        --name) VM_NAME="$2" ;;
        --memory) MEMORY_MB="$2" ;;
        --vcpus) VCPUS="$2" ;;
        --disk-size) DISK_SIZE_GB="$2" ;;
        --network) NETWORK="$2" ;;
      esac
      shift 2
      ;;
    *) usage >&2; die "不明なオプション: $1" ;;
  esac
done

case "${VERSION}" in
  ''|2022|2025) ;;
  *) die "--version には 2022 または 2025 を指定してください: ${VERSION}" ;;
esac
require_positive_int --memory "${MEMORY_MB}"
require_positive_int --vcpus "${VCPUS}"
require_positive_int --disk-size "${DISK_SIZE_GB}"
[ -d "${WORKDIR}" ] || die "作業ディレクトリが存在しません: ${WORKDIR}"
WORKDIR="$(cd "${WORKDIR}" && pwd)"

BUILD_DIR="${WORKDIR}/build_qcow2"
ISOS_DIR="${BUILD_DIR}/isos"
UNATTEND_DIR="${BUILD_DIR}/unattend"
VIRTIO_ISO="${ISOS_DIR}/virtio-win.iso"
AUTOUNATTEND="${UNATTEND_DIR}/autounattend.xml"
CLOUDBASE_MSI="${UNATTEND_DIR}/CloudbaseInitSetup_1_1_8_x64.msi"

WIN2K22_ISO="${WIN2K22_ISO:-${ISOS_DIR}/SERVER_EVAL_x64FRE_ja-jp.iso}"
WIN2K25_ISO="${WIN2K25_ISO:-${ISOS_DIR}/26100.32230.260111-0550.lt_release_svc_refresh_SERVER_EVAL_x64FRE_ja-jp.iso}"

for command_name in genisoimage virt-install virsh sudo; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    die "${command_name} command not found"
  fi
done

echo "1. 実行前の準備を確認する"

if [ -z "${VERSION}" ]; then
  if [ -f "${WIN2K22_ISO}" ] && [ -f "${WIN2K25_ISO}" ]; then
    die "Windows Server 2022 と 2025 の ISO が両方存在します。--version 2022|2025 で指定してください。"
  elif [ -f "${WIN2K22_ISO}" ]; then
    VERSION="2022"
  elif [ -f "${WIN2K25_ISO}" ]; then
    VERSION="2025"
  fi
fi

case "${VERSION}" in
  2022)
    OS_VARIANT="win2k22"
    DRIVER_TAG="2k22"
    INSTALL_ISO="${WIN2K22_ISO}"
    ;;
  2025)
    OS_VARIANT="win2k25"
    DRIVER_TAG="2k25"
    INSTALL_ISO="${WIN2K25_ISO}"
    ;;
  *)
    echo "Windows Server インストール用 ISO が見つかりません。" >&2
    echo "次のいずれかを自己責任で入手して配置してください。" >&2
    echo "  Win2k22: ${WIN2K22_ISO}" >&2
    echo "  Win2k25: ${WIN2K25_ISO}" >&2
    exit 1
    ;;
esac

if [ ! -f "${INSTALL_ISO}" ]; then
  die "Windows Server ${VERSION} 用 ISO が見つかりません: ${INSTALL_ISO}"
fi
echo "  Windows Server ${VERSION} 用 ISO を使用します: ${INSTALL_ISO}"

VM_NAME="${VM_NAME:-windows${VERSION}-build}"
VIRSH=(sudo virsh -c qemu:///system)
VM_EXISTS=false
if "${VIRSH[@]}" dominfo "${VM_NAME}" >/dev/null 2>&1; then
  [ "${FORCE}" = true ] || die "VM ${VM_NAME} は既に存在します。--force で作り直すか、--name で別名を指定してください。"
  VM_EXISTS=true
fi

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

if [ "${VM_EXISTS}" = true ]; then
  echo "  既存の VM ${VM_NAME} を停止・削除します (--force)"
  "${VIRSH[@]}" destroy "${VM_NAME}" >/dev/null 2>&1 || true
  "${VIRSH[@]}" undefine "${VM_NAME}" --nvram
fi

echo "2. autounattend.xml をテンプレートから生成する (ドライバパス: ${DRIVER_TAG})"
mkdir -p "${UNATTEND_DIR}"
sed "s/2k22/${DRIVER_TAG}/g" "${TEMPLATE_XML}" > "${AUTOUNATTEND}"

# 前回実行分が残っていれば削除してから作り直す
# （VM がインストール中に参照するため、実行後には削除しない）
ASSETS_ISO="${ISOS_DIR}/assets.iso"
rm -f "${ASSETS_ISO}"
echo "3. assets.iso イメージを作成する"
genisoimage -output "${ASSETS_ISO}" -volid ASSETS -joliet -rock "${UNATTEND_DIR}"

echo "4. 実行開始"
mkdir -p "${BUILD_DIR}"
# 前回実行で作成された qcow2 を削除する（root 所有の場合があるため sudo を使う）
sudo find "${BUILD_DIR}" -type f -name '*.qcow2' -print -delete
QCOW2_PATH="${BUILD_DIR}/${VM_NAME}.qcow2"

sudo virt-install \
    --name "${VM_NAME}" \
    --memory "${MEMORY_MB}" \
    --vcpus "${VCPUS}" \
    --os-variant "${OS_VARIANT}" \
    --boot uefi \
    --tpm backend.type=emulator,backend.version=2.0,model=tpm-crb \
    --disk path="${QCOW2_PATH}",bus=scsi,format=qcow2,size="${DISK_SIZE_GB}" \
    --controller type=scsi,model=virtio-scsi \
    --network network="${NETWORK}",model=virtio \
    --graphics spice \
    --video model.type=qxl \
    --cdrom "${INSTALL_ISO}" \
    --disk "${VIRTIO_ISO}",device=cdrom \
    --disk "${ASSETS_ISO}",device=cdrom \
    --noautoconsole

echo "完了: ${QCOW2_PATH} の作成を開始しました。"
echo "グラフィカルコンソール (SPICE) で進行状況を確認してください。"
echo "  virt-viewer -c qemu:///system ${VM_NAME}"
echo "  接続情報の確認: sudo virsh -c qemu:///system domdisplay ${VM_NAME}"
