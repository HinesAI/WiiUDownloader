#!/usr/bin/env bash
# Fetch and normalize the title database for WiiUDownloader builds.
set -euo pipefail

UA="User-Agent: NUSspliBuilder/2.1"
URL="https://napi.v10lator.de/db?t=go"
OUT="${1:-db.go}"

download() {
  local extra_flags=()
  if [[ "${1:-}" == "insecure" ]]; then
    extra_flags=(-k)
  fi
  curl -fsSL "${extra_flags[@]}" -H "$UA" -o "$OUT" "$URL"
}

if ! download; then
  echo "WARN: secure download of title DB failed; retrying with TLS verify disabled" >&2
  download insecure
fi

# BSD sed (macOS) needs sed -i ''; GNU sed accepts sed -i
if [[ "$(uname -s)" == "Darwin" ]]; then
  sed_i() { sed -i '' "$@"; }
else
  sed_i() { sed -i "$@"; }
fi

if grep -q 'var titleEntry =' "$OUT"; then
  if grep -q 'type TitleEntry struct' "$OUT"; then
    sed_i '/type TitleEntry struct/,/}/d' "$OUT"
  fi
  sed_i 's/var titleEntry =/func init() { TitleDatabase =/' "$OUT"
  echo '}' >> "$OUT"
fi

echo "Prepared $OUT ($(wc -c < "$OUT") bytes)"
