#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/ci_release_common.sh"
: "${GH_TOKEN:?GH_TOKEN is required}"
: "${GH_REPO:?GH_REPO is required}"
test -s dist/release_notes.md
(cd dist/release && shasum -a 256 -c SHA256SUMS)
prerelease=false
if [[ "${RELEASE_VERSION%%+*}" == *-* ]]; then prerelease=true; fi

if gh release view "$RELEASE_TAG" --repo "$GH_REPO" >/dev/null 2>&1; then
  remote_commit="$(gh api "repos/$GH_REPO/commits/$RELEASE_TAG" --jq '.sha')"
  if [[ "$remote_commit" != "$RELEASE_COMMIT" ]]; then
    echo "Release tag points to a different commit; bump VERSION" >&2
    exit 1
  fi
else
  gh release create "$RELEASE_TAG" --repo "$GH_REPO" --target "$RELEASE_COMMIT" \
    --title "Release $RELEASE_TAG" --notes-file dist/release_notes.md --draft
fi
gh release upload "$RELEASE_TAG" --repo "$GH_REPO" \
  dist/release/dyno-"$RELEASE_VERSION"-*.tar.gz dist/release/SHA256SUMS --clobber
gh release edit "$RELEASE_TAG" --repo "$GH_REPO" --title "Release $RELEASE_TAG" \
  --notes-file dist/release_notes.md --draft=false --prerelease="$prerelease"
