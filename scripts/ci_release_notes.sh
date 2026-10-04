#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/ci_release_common.sh"

mkdir -p dist
previous_tag=""
while IFS= read -r tag; do
  if [[ "$tag" != "$RELEASE_TAG" ]]; then
    previous_tag="$tag"
    break
  fi
done < <(git tag --merged "$RELEASE_COMMIT" --list 'v[0-9]*' --sort=-version:refname)
range="$RELEASE_COMMIT"
if [[ -n "$previous_tag" ]]; then
  range="$previous_tag..$RELEASE_COMMIT"
fi
{
  printf '## %s - %s\n\n' "$RELEASE_TAG" "$(date -u +%Y-%m-%d)"
  if [[ -n "$previous_tag" ]]; then
    printf 'Changes since %s:\n\n' "$previous_tag"
  else
    printf 'Initial release:\n\n'
  fi
  git log --no-merges --pretty=format:'- %s (%h)' "$range"
  printf '\n'
} > dist/release_notes.md
printf 'RELEASE_VERSION=%s\nRELEASE_TAG=%s\nRELEASE_COMMIT=%s\n' \
  "$RELEASE_VERSION" "$RELEASE_TAG" "$RELEASE_COMMIT" > dist/release.env
cat dist/release_notes.md
