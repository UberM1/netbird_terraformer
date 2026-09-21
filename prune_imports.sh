#!/usr/bin/env bash
# Drops import blocks for resources already tracked in the Terraform state.
#
# Terraform fails the whole plan if an import block targets a resource it
# already manages, so the generated imports.tf has to be pruned before use.
#
# Usage:
#   cd <your terraform project directory>
#   terraform state list > /tmp/nb-state.txt
#   bash /path/to/prune_imports.sh imports.tf /tmp/nb-state.txt

set -euo pipefail

IMPORTS="${1:-imports.tf}"
STATE_LIST="${2:-}"

if [ -z "$STATE_LIST" ]; then
  echo "usage: $0 <imports.tf> <state-list-file>" >&2
  echo "  generate the state list with: terraform state list > state.txt" >&2
  exit 1
fi

[ -f "$IMPORTS" ] || { echo "not found: $IMPORTS" >&2; exit 1; }
[ -f "$STATE_LIST" ] || { echo "not found: $STATE_LIST" >&2; exit 1; }

TMP="$(mktemp)"
kept=0
dropped=0
block=""
address=""

flush() {
  [ -z "$block" ] && return
  if grep -qxF "$address" "$STATE_LIST"; then
    dropped=$((dropped + 1))
  else
    printf '%s\n\n' "$block" >> "$TMP"
    kept=$((kept + 1))
  fi
  block=""
  address=""
}

# Keep the header comments, then rebuild the file block by block.
sed -n '1,/^$/p' "$IMPORTS" > "$TMP"

while IFS= read -r line; do
  case "$line" in
    "import {")
      block="$line"
      ;;
    "}")
      if [ -n "$block" ]; then
        block="$block"$'\n'"$line"
        flush
      fi
      ;;
    *)
      if [ -n "$block" ]; then
        block="$block"$'\n'"$line"
        case "$line" in
          *"to = "*) address="${line#*to = }" ;;
        esac
      fi
      ;;
  esac
done < "$IMPORTS"

mv "$TMP" "$IMPORTS"

echo "kept $kept import blocks, dropped $dropped already in state"
