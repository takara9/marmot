#!/usr/bin/env bash
#
# Logical volume cleanup script
# 
set -euo pipefail

tmpfile=$(mktemp)
sudo lvs --reportformat json  |jq -r '.report[].lv[] | "\(.vg_name)/\(.lv_name)"' > "$tmpfile"


cat "$tmpfile" | while read -r line; do
  lv_path="$line"
  echo "Deleting logical volume $lv_path"
  sudo lvremove -y "$lv_path"
done

rm -f "$tmpfile"