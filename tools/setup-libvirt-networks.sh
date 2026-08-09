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

for command_name in ovs-appctl ovs-vsctl ip; do
  if ! command -v "${command_name}" >/dev/null 2>&1; then
    echo "${command_name} command not found" >&2
    exit 1
  fi
done

ensure_service_if_exists() {
  local unit_name="$1"
  local required="${2:-false}"
  if systemctl list-unit-files | grep -q "^${unit_name}\\.service"; then
    sudo systemctl enable "${unit_name}.service" || true
    if ! sudo systemctl start "${unit_name}.service"; then
      if [[ "${required}" == "true" ]]; then
        return 1
      fi
      echo "optional service ${unit_name}.service could not be started" >&2
    fi
  fi
}

ensure_ovn_ovs_runtime() {
  ensure_service_if_exists openvswitch-switch true
  ensure_service_if_exists ovsdb-server true
  ensure_service_if_exists ovs-vswitchd true
  ensure_service_if_exists ovn-central
  ensure_service_if_exists ovn-northd
  ensure_service_if_exists ovn-controller
  ensure_service_if_exists ovn-host
}

print_ovs_diagnostics() {
  echo "OVS runtime diagnostics:" >&2
  sudo systemctl --no-pager --full status openvswitch-switch ovsdb-server ovs-vswitchd 2>&1 || true
  sudo ovs-vsctl show 2>&1 || true
  sudo ovs-appctl -t ovs-vswitchd dpif/show 2>&1 || true
  ip -details link show 2>&1 || true
}

wait_for_linux_bridge() {
  local bridge_name="$1"
  local attempt

  for attempt in $(seq 1 30); do
    if ip link show "${bridge_name}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

create_ovs_bridge() {
  local bridge_name="$1"

  sudo ovs-vsctl --if-exists del-br "${bridge_name}"
  sudo ovs-vsctl --may-exist add-br "${bridge_name}"
  wait_for_linux_bridge "${bridge_name}"
}

restart_ovs_runtime() {
  if systemctl list-unit-files | grep -q '^openvswitch-switch\.service'; then
    sudo systemctl restart openvswitch-switch.service
    return
  fi
  sudo systemctl restart ovsdb-server.service ovs-vswitchd.service
}

ensure_ovs_bridge() {
  local bridge_name="$1"
  if sudo ovs-vsctl br-exists "${bridge_name}" && ip link show "${bridge_name}" >/dev/null 2>&1; then
    echo "${bridge_name} already exists"
    return 0
  fi

  echo "creating ovs bridge ${bridge_name}"
  if ! create_ovs_bridge "${bridge_name}"; then
    echo "${bridge_name} Linux device was not created; restarting OVS runtime" >&2
    restart_ovs_runtime
    sudo ovs-appctl -t ovs-vswitchd version >/dev/null
    if create_ovs_bridge "${bridge_name}"; then
      return 0
    fi
    echo "${bridge_name} exists in OVSDB but its Linux device was not created" >&2
    print_ovs_diagnostics
    return 1
  fi
}

cleanup_stale_marmot_bridges() {
  local bridge_name
  local deleted=0

  while IFS= read -r bridge_name; do
    if [[ "${bridge_name}" =~ ^br-[0-9a-f]{5}$ ]]; then
      echo "deleting stale Marmot OVS bridge ${bridge_name}"
      sudo ovs-vsctl --if-exists del-br "${bridge_name}"
      deleted=$((deleted + 1))
    fi
  done < <(sudo ovs-vsctl list-br)
  echo "deleted ${deleted} stale Marmot OVS bridge(s)"
}

probe_dynamic_ovs_bridge() {
  local bridge_name="mkeprb-$$"

  echo "probing dynamic OVS bridge ${bridge_name}"
  if ! create_ovs_bridge "${bridge_name}"; then
    echo "dynamic OVS bridge probe failed" >&2
    print_ovs_diagnostics
    return 1
  fi
  sudo ovs-vsctl --if-exists del-br "${bridge_name}"
  if ip link show "${bridge_name}" >/dev/null 2>&1; then
    echo "dynamic OVS bridge probe device remains after deletion: ${bridge_name}" >&2
    print_ovs_diagnostics
    return 1
  fi
}

ensure_ovn_ovs_runtime
if ! sudo ovs-appctl -t ovs-vswitchd version >/dev/null 2>&1; then
  echo "ovs-vswitchd is not responding" >&2
  print_ovs_diagnostics
  exit 1
fi
ensure_ovs_bridge "ovsbr0"
cleanup_stale_marmot_bridges
probe_dynamic_ovs_bridge

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