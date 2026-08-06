#!/usr/bin/env bash
# Compare this fork against upstream Xpl0itU/WiiUDownloader and print a summary.
# Used by the monthly CI check and for local review.
set -euo pipefail

UPSTREAM_REPO="${UPSTREAM_REPO:-Xpl0itU/WiiUDownloader}"
UPSTREAM_REF="${UPSTREAM_REF:-main}"
REMOTE_NAME="${REMOTE_NAME:-upstream}"

cd "$(git rev-parse --show-toplevel)"

if ! git remote get-url "$REMOTE_NAME" >/dev/null 2>&1; then
  git remote add "$REMOTE_NAME" "https://github.com/${UPSTREAM_REPO}.git"
fi

echo "Fetching ${UPSTREAM_REPO}@${UPSTREAM_REF}..."
git fetch --quiet "$REMOTE_NAME" "$UPSTREAM_REF"

OUR_HEAD="$(git rev-parse HEAD)"
UP_HEAD="$(git rev-parse "${REMOTE_NAME}/${UPSTREAM_REF}")"
MERGE_BASE="$(git merge-base HEAD "${REMOTE_NAME}/${UPSTREAM_REF}" || true)"

echo "our_head=${OUR_HEAD}"
echo "upstream_head=${UP_HEAD}"
echo "merge_base=${MERGE_BASE:-none}"

if [[ -z "${MERGE_BASE}" ]]; then
  echo "status=no-common-ancestor"
  echo "NOTE: No merge-base with upstream (histories may have diverged). Review manually."
  exit 0
fi

AHEAD="$(git rev-list --count "${MERGE_BASE}..${REMOTE_NAME}/${UPSTREAM_REF}")"
BEHIND="$(git rev-list --count "${MERGE_BASE}..HEAD")"

echo "upstream_commits_since_base=${AHEAD}"
echo "our_commits_since_base=${BEHIND}"

if [[ "$AHEAD" -eq 0 ]]; then
  echo "status=up-to-date"
  echo "No new upstream commits since the common ancestor."
  exit 0
fi

echo "status=updates-available"
echo
echo "=== Upstream commits not in our tree (${AHEAD}) ==="
git log --oneline --no-decorate "${MERGE_BASE}..${REMOTE_NAME}/${UPSTREAM_REF}" | head -50
if [[ "$AHEAD" -gt 50 ]]; then
  echo "... ($((AHEAD - 50)) more)"
fi

echo
echo "=== Diffstat (upstream changes since merge-base) ==="
git diff --stat "${MERGE_BASE}...${REMOTE_NAME}/${UPSTREAM_REF}" | tail -40

echo
echo "=== Decision guide ==="
echo "Merge wholesale when changes are small, compatible, and touch shared core paths."
echo "Port selectively when upstream churn conflicts with HinesAI UI/branding/CI, or"
echo "when only a bugfix/feature is useful — cherry-pick or re-implement that piece."
