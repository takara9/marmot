#!/usr/bin/env bash
#
# OVS bridge cleanup script
# 
set -euo pipefail

tmpfile=$(mktemp)
ovs-vsctl list-br |grep -v br-int > "$tmpfile"

cat "$tmpfile" | while read -r line; do
  bridge_name="$line"
  echo "Deleting  $bridge_name"
  ovs-vsctl del-br "$bridge_name"
done

rm -f "$tmpfile"