#!/usr/bin/env bash
set -euo pipefail

raw="$(curl -s -o /dev/null -w '%{http_code}' "$@" 2>/dev/null || true)"
code="$(printf '%s' "$raw" | tr -cd '0-9' | cut -c1-3)"

if [ "${#code}" -ne 3 ]; then
    code="000"
fi

printf '%s\n' "$code"
