#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist"

USER_NAME="root"
TARGET_HOSTS=(
  "192.168.1.201"
  "192.168.1.202"
  "192.168.1.203"
)

usage() {
  cat <<EOF
Usage: $(basename "$0") [-u USER] [DEB_FILE]

Description:
  Copy the package created by make package to target hosts,
  then install it with dpkg -i.

Arguments:
  DEB_FILE   Optional path to a .deb file. If omitted, the newest
             marmot*.deb in ${DIST_DIR} is used.

Options:
  -u USER    SSH/SCP user name (default: root)
  -h         Show this help

Examples:
  $(basename "$0")
  $(basename "$0") ${DIST_DIR}/marmot_v0.12.0_amd64.deb
  $(basename "$0") -u ubuntu ${DIST_DIR}/marmot_v0.12.0_amd64.deb
EOF
}

while getopts ":u:h" opt; do
  case "${opt}" in
    u)
      USER_NAME="${OPTARG}"
      ;;
    h)
      usage
      exit 0
      ;;
    :) 
      echo "Error: -${OPTARG} requires an argument." >&2
      usage
      exit 1
      ;;
    \?)
      echo "Error: invalid option -${OPTARG}" >&2
      usage
      exit 1
      ;;
  esac
done
shift $((OPTIND - 1))

if [[ $# -gt 1 ]]; then
  echo "Error: too many arguments." >&2
  usage
  exit 1
fi

if [[ $# -eq 1 ]]; then
  DEB_PATH="$1"
else
  DEB_PATH="$(ls -1t "${DIST_DIR}"/marmot*.deb 2>/dev/null | head -n 1 || true)"
fi

if [[ -z "${DEB_PATH}" ]]; then
  echo "Error: no deb file found in ${DIST_DIR}." >&2
  echo "Run make package first, or pass DEB_FILE explicitly." >&2
  exit 1
fi

if [[ ! -f "${DEB_PATH}" ]]; then
  echo "Error: deb file not found: ${DEB_PATH}" >&2
  exit 1
fi

DEB_BASENAME="$(basename "${DEB_PATH}")"
REMOTE_PATH="/tmp/${DEB_BASENAME}"

echo "Using deb file: ${DEB_PATH}"
echo "Target hosts: ${TARGET_HOSTS[*]}"
echo "SSH user: ${USER_NAME}"

failed_hosts=()

for host in "${TARGET_HOSTS[@]}"; do
  echo ""
  echo "==== ${host}: transfer ===="
  if ! scp "${DEB_PATH}" "${USER_NAME}@${host}:${REMOTE_PATH}"; then
    echo "${host}: scp failed" >&2
    failed_hosts+=("${host}")
    continue
  fi

  echo "==== ${host}: install (dpkg -i) ===="
  if ! ssh "${USER_NAME}@${host}" "dpkg -i '${REMOTE_PATH}'"; then
    echo "${host}: dpkg -i failed" >&2
    failed_hosts+=("${host}")
    continue
  fi

  echo "${host}: done"
done

echo ""
if [[ ${#failed_hosts[@]} -gt 0 ]]; then
  echo "Finished with failures on: ${failed_hosts[*]}" >&2
  exit 1
fi

echo "Finished successfully on all hosts."
