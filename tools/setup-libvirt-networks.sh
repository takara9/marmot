#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

networks=(
  "default"
  "host-bridge"
  "ovs-network"
)

if ! command -v virsh >/dev/null 2>&1; then
  echo "virsh command not found" >&2
  exit 1
fi

active_networks="$(virsh net-list --name)"

ensure_network() {
  local network_name="$1"
  local xml_file="${SCRIPT_DIR}/${network_name}.xml"

  if printf '%s\n' "${active_networks}" | grep -Fxq "${network_name}"; then
    echo "${network_name} is already active"
    return
  fi

  if [[ ! -f "${xml_file}" ]]; then
    echo "network definition file not found: ${xml_file}" >&2
    exit 1
  fi

  if virsh net-info "${network_name}" >/dev/null 2>&1; then
    echo "${network_name} is defined but inactive; starting it"
  else
    echo "${network_name} is not defined; defining it from ${xml_file}"
    virsh net-define "${xml_file}"
  fi

  virsh net-start "${network_name}"
  virsh net-autostart "${network_name}"
}

for network_name in "${networks[@]}"; do
  ensure_network "${network_name}"
done

echo "active libvirt networks:"
virsh net-list