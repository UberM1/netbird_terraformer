#!/usr/bin/env bash
# Reconciles Terraform state with the configuration WITHOUT touching NetBird.
#
# Import blocks in imports.tf only execute during `terraform apply`, which would
# also run every create/update/destroy in the plan. `terraform import` and
# `terraform state mv` are state-only: they read from the API and write state,
# and never call Create/Update/Delete. That is the difference this script exists
# for -- it closes the gap between state and config while leaving the live
# infrastructure exactly as it is.
#
# Reads:
#   moved.tf   -> a `terraform state mv` per moved block (renames)
#   imports.tf -> a `terraform import` per import block
#
# Both steps are idempotent: anything already in the desired state is skipped,
# so the script is safe to re-run after a partial or interrupted run.
#
# Usage:
#   cd <your terraform project directory>
#   bash /path/to/reconcile_state.sh --dry-run
#   bash /path/to/reconcile_state.sh
#
# Expects the backend and provider credentials to already be exported
# (TF_HTTP_*, TF_VAR_netbird_token).

set -uo pipefail

DRY_RUN=false
IMPORTS="imports.tf"
MOVED="moved.tf"

while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY_RUN=true ;;
    --imports) IMPORTS="$2"; shift ;;
    --moved)   MOVED="$2"; shift ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done

[ -f "$IMPORTS" ] || { echo "not found: $IMPORTS (run from the project directory)" >&2; exit 1; }

run() {
  if [ "$DRY_RUN" = true ]; then
    echo "  DRY-RUN: $*"
    return 0
  fi
  "$@"
}

echo "Reading current state..."
STATE_LIST="$(mktemp)"
trap 'rm -f "$STATE_LIST"' EXIT
if ! terraform state list > "$STATE_LIST" 2>/dev/null; then
  echo "ERROR: could not read state. Is the backend initialised and are TF_HTTP_* exported?" >&2
  exit 1
fi
echo "  $(wc -l < "$STATE_LIST" | tr -d ' ') resources currently in state"
echo

in_state() { grep -qxF "$1" "$STATE_LIST"; }

moved_ok=0; moved_skip=0; moved_fail=0

if [ -f "$MOVED" ]; then
  echo "== Renames (terraform state mv) =="
  # Pair each `from =` with the `to =` that follows it.
  while read -r from to; do
    if in_state "$to"; then
      echo "  skip (already at target): $to"
      moved_skip=$((moved_skip + 1))
    elif ! in_state "$from"; then
      echo "  skip (source not in state): $from"
      moved_skip=$((moved_skip + 1))
    else
      echo "  mv $from -> $to"
      if run terraform state mv "$from" "$to"; then
        moved_ok=$((moved_ok + 1))
      else
        echo "  FAILED: $from -> $to" >&2
        moved_fail=$((moved_fail + 1))
      fi
    fi
  done < <(awk '
    /^[[:space:]]*from[[:space:]]*=/ { f=$3 }
    /^[[:space:]]*to[[:space:]]*=/   { if (f != "") { print f, $3; f="" } }
  ' "$MOVED")
  echo
fi

ok=0; skipped=0; failed=0
FAILED_LIST="$(mktemp)"

echo "== Imports (terraform import) =="
# Each import block is `to = <address>` followed by `id = "<id>"`.
while read -r addr id; do
  if in_state "$addr"; then
    echo "  skip (already in state): $addr"
    skipped=$((skipped + 1))
    continue
  fi
  echo "  import $addr <- $id"
  if run terraform import "$addr" "$id" >/dev/null 2>&1; then
    ok=$((ok + 1))
  else
    echo "  FAILED: $addr <- $id" >&2
    echo "$addr $id" >> "$FAILED_LIST"
    failed=$((failed + 1))
  fi
done < <(awk '
  /^[[:space:]]*to[[:space:]]*=/ { a=$3 }
  /^[[:space:]]*id[[:space:]]*=/ { if (a != "") { gsub(/"/,"",$3); print a, $3; a="" } }
' "$IMPORTS")

echo
echo "=================================================="
echo "renames : $moved_ok moved, $moved_skip skipped, $moved_fail failed"
echo "imports : $ok imported, $skipped skipped, $failed failed"
if [ "$failed" -gt 0 ]; then
  echo
  echo "Failed imports (re-runnable):"
  cat "$FAILED_LIST"
fi
rm -f "$FAILED_LIST"
echo "=================================================="
echo
echo "Next: terraform plan. A clean reconcile leaves no 'to import' entries."

[ "$failed" -eq 0 ] && [ "$moved_fail" -eq 0 ]
