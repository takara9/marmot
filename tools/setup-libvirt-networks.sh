#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

networks=(
  "default"
  "host-bridge"
)

if ! command -v virsh >/dev/null 2>&1; then
  echo "virsh command not found" >&2
  exit 1
fi

ensure_service_if_exists() {
  local unit_name="$1"
  if systemctl list-unit-files | grep -q "^${unit_name}\\.service"; then
    sudo systemctl enable "${unit_name}.service" || true
    sudo systemctl start "${unit_name}.service" || true
  fi
}

ensure_ovn_ovs_runtime() {
  ensure_service_if_exists openvswitch-switch
  ensure_service_if_exists ovsdb-server
  ensure_service_if_exists ovs-vswitchd
  ensure_service_if_exists ovn-central
  ensure_service_if_exists ovn-northd
  ensure_service_if_exists ovn-controller
  ensure_service_if_exists ovn-host
}

ensure_ovs_bridge() {
  local bridge_name="$1"
  if ! command -v ovs-vsctl >/dev/null 2>&1; then
    echo "ovs-vsctl command not found" >&2
    return 0
  fi
  if ovs-vsctl br-exists "${bridge_name}"; then
    echo "${bridge_name} already exists"
    return 0
  fi
  echo "creating ovs bridge ${bridge_name}"
  sudo ovs-vsctl --may-exist add-br "${bridge_name}"
}

ensure_ovn_ovs_runtime
ensure_ovs_bridge "ovsbr0"

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