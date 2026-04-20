#!/usr/bin/env bash

set -euo pipefail

networks=(
  "default"
  "host-bridge"
  "ovs-network"
)

tmpfile_active="$(mktemp)"
virsh net-list --name > "${tmpfile_active}"

# まずはactiveなネットワークを停止して定義も削除する
while read -r network_name2; do
    echo "checking ${network_name2}"
    for network_name in "${networks[@]}"; do
        if [[ "${network_name2}" == "${network_name}" ]]; then
            virsh net-destroy "${network_name}"
            sleep 2
            virsh net-undefine "${network_name}"
            break
        fi
    done
done <"${tmpfile_active}"

tmpfile_inactive="$(mktemp)"
virsh net-list --all --name > "${tmpfile_inactive}"

# 次にinactiveなネットワークも定義を削除する
while read -r network_name2; do
    echo "checking ${network_name2}"
    for network_name in "${networks[@]}"; do
        if [[ "${network_name2}" == "${network_name}" ]]; then
            virsh net-undefine "${network_name}"
            break
        fi
    done
done <"${tmpfile_inactive}"

virsh net-list --all

rm -f "${tmpfile_active}" "${tmpfile_inactive}"