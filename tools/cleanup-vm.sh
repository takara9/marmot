#!/usr/bin/env bash
#
# VM cleanup script
# 
set -euo pipefail

tmpfile=$(mktemp)

virsh list --all | awk 'NR>2 && NF {print $2}' | xargs -I{} virsh undefine {}

rm -f "$tmpfile"