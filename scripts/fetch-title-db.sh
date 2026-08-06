#!/usr/bin/env bash
# Fetch and normalize the title database for WiiUDownloader builds.
set -euo pipefail

UA="User-Agent: NUSspliBuilder/2.1"
URL="https://napi.v10lator.de/db?t=go"
OUT="${1:-db.go}"

if ! curl -fsSL -H "$UA" -o "$OUT" "$URL"; then
  echo "WARN: secure download of title DB failed; retrying with TLS verify disabled" >&2
  curl -kfsSL -H "$UA" -o "$OUT" "$URL"
fi

if grep -q 'var titleEntry =' "$OUT"; then
  tmp="$(mktemp)"
  # Drop an embedded TitleEntry struct if present, then wrap the slice in init().
  awk '
    /type TitleEntry struct/ { skip=1; next }
    skip && /^[[:space:]]*}[[:space:]]*$/ { skip=0; next }
    skip { next }
    { print }
  ' "$OUT" | sed 's/var titleEntry =/func init() { TitleDatabase =/' >"$tmp"
  echo '}' >>"$tmp"
  mv "$tmp" "$OUT"
fi

bytes="$(wc -c <"$OUT" | tr -d ' ')"
echo "Prepared $OUT ($bytes bytes)"
