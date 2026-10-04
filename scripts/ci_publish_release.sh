#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/ci_release_common.sh"
: "${CI_API_V4_URL:?CI_API_V4_URL is required}"
: "${CI_PROJECT_ID:?CI_PROJECT_ID is required}"
: "${CI_JOB_TOKEN:?CI_JOB_TOKEN is required}"
test -s dist/release_notes.md
(cd dist/release && shasum -a 256 -c SHA256SUMS)

api="$CI_API_V4_URL/projects/$CI_PROJECT_ID"
tag_path="$(jq -nr --arg tag "$RELEASE_TAG" '$tag | @uri')"
package_url="$api/packages/generic/dyno/$RELEASE_VERSION"
response="$(mktemp)"
trap 'rm -f "$response"' EXIT

# Only a 404 means missing. Authentication and server errors must stop publication.
get_optional() {
  local status
  status="$(curl --silent --show-error --output "$response" --write-out '%{http_code}' \
    --header "JOB-TOKEN: $CI_JOB_TOKEN" "$1")"
  case "$status" in
    200) return 0 ;;
    404) return 1 ;;
    *) echo "GitLab API returned HTTP $status" >&2; exit 1 ;;
  esac
}

if get_optional "$api/repository/tags/$tag_path"; then
  if [[ "$(jq -r '.commit.id' "$response")" != "$RELEASE_COMMIT" ]]; then
    echo "Release tag points to a different commit; bump VERSION" >&2
    exit 1
  fi
fi
method=POST
endpoint="$api/releases"
existing_links='[]'
if get_optional "$api/releases/$tag_path"; then
  if [[ "$(jq -r '.commit.id' "$response")" != "$RELEASE_COMMIT" ]]; then
    echo "Existing release belongs to a different commit" >&2
    exit 1
  fi
  method=PUT
  endpoint="$api/releases/$tag_path"
  existing_links="$(curl --silent --show-error --fail --header "JOB-TOKEN: $CI_JOB_TOKEN" \
    "$api/releases/$tag_path/assets/links?per_page=100")"
fi

links='[]'
for file in dist/release/dyno-"$RELEASE_VERSION"-*.tar.gz dist/release/SHA256SUMS; do
  test -s "$file"
  name="$(basename "$file")"
  url="$package_url/$name"
  echo "Uploading $name"
  curl --silent --show-error --fail --header "JOB-TOKEN: $CI_JOB_TOKEN" \
    --upload-file "$file" "$url" >/dev/null
  links="$(jq -c --arg name "$name" --arg url "$url" \
    '. + [{name: $name, url: $url, link_type: "package"}]' <<< "$links")"
done
payload="$(jq -n --arg tag "$RELEASE_TAG" --arg ref "$RELEASE_COMMIT" \
  --rawfile notes dist/release_notes.md --argjson links "$links" \
  '{tag_name: $tag, ref: $ref, name: ("Release " + $tag), description: $notes, assets: {links: $links}}')"
if [[ "$method" == PUT ]]; then
  payload="$(jq -c 'del(.assets, .tag_name, .ref)' <<< "$payload")"
fi
curl --silent --show-error --fail --request "$method" \
  --header "JOB-TOKEN: $CI_JOB_TOKEN" --header 'Content-Type: application/json' \
  --data "$payload" "$endpoint" >/dev/null

# The release update endpoint does not update existing asset links.
if [[ "$method" == PUT ]]; then
  while IFS= read -r link; do
    name="$(jq -r '.name' <<< "$link")"
    id="$(jq -r --arg name "$name" '.[] | select(.name == $name) | .id' <<< "$existing_links")"
    link_method=POST
    link_endpoint="$api/releases/$tag_path/assets/links"
    if [[ -n "$id" ]]; then
      link_method=PUT
      link_endpoint="$link_endpoint/$id"
    fi
    curl --silent --show-error --fail --request "$link_method" \
      --header "JOB-TOKEN: $CI_JOB_TOKEN" --header 'Content-Type: application/json' \
      --data "$link" "$link_endpoint" >/dev/null
  done < <(jq -c '.[]' <<< "$links")
fi
echo "Published GitLab release $RELEASE_TAG"
