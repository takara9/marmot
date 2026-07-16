#!/bin/bash -e

echo "Ubuntu 24.04 (noble) のcloud imageをダウンロードしてカスタマイズする"
QCOW2POOL="/var/lib/marmot/volumes"
IMAGE="ubuntu-24.04-server-cloudimg-amd64.img"
IMAGE_TEMPLATE="ubuntu-24.04-template.qcow2"

if [ -d "${QCOW2POOL}" ]; then
  echo "${QCOW2POOL} ディレクトリは存在します。"
else
  echo "${QCOW2POOL} ディレクトリを作成します。"
  mkdir -p ${QCOW2POOL}
fi
cd ${QCOW2POOL}

echo "http://hmc/${IMAGE} にアクセスしてダウンロードの成否を確認する"
if curl -fsSI "http://hmc/${IMAGE}" >/dev/null; then
  echo "ダウンロード可能: http://hmc/${IMAGE}"
  curl -fSL -O "http://hmc/${IMAGE}"
else
  echo "ダウンロード不可: http://hmc/${IMAGE}。インターネットからcloud imageをダウンロードします。"
  curl -fSL -O "https://cloud-images.ubuntu.com/releases/noble/release/${IMAGE}"
fi

