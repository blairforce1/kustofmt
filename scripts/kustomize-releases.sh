#!/usr/bin/env bash
# List published kustomize CLI releases, oldest first.
#
#   scripts/kustomize-releases.sh          -> 5.0.0 5.0.1 ... 5.8.1 (one per line)
#   scripts/kustomize-releases.sh 5.6.0    -> only 5.6.0 and newer
#
# The kustomize repository is a monorepo: it tags `api/v*`, `kyaml/v*` and
# `cmd/config/v*` alongside the CLI. Only `kustomize/v*` tags are CLI releases,
# which is what the compatibility matrix tracks.
#
# Drafts and prereleases are excluded: the matrix records what a user can
# actually install and pin.
set -euo pipefail

readonly API=https://api.github.com/repos/kubernetes-sigs/kustomize/releases

die() {
	echo "kustomize-releases: $*" >&2
	exit 2
}

# fetch pages the releases API. A token is optional -- the repository is public
# -- but raises the rate limit, and CI always has one.
fetch() {
	local page=1 body auth=()
	[ -n "${GH_TOKEN:-${GITHUB_TOKEN:-}}" ] &&
		auth=(-H "Authorization: Bearer ${GH_TOKEN:-$GITHUB_TOKEN}")
	while :; do
		body=$(curl -fsSL --max-time 30 "${auth[@]}" \
			-H "Accept: application/vnd.github+json" \
			"$API?per_page=100&page=$page" 2>/dev/null) ||
			die "cannot reach the GitHub releases API (no network, or rate limited)"
		[ "$(printf '%s' "$body" | tr -d '[:space:]')" = "[]" ] && break
		printf '%s\n' "$body"
		page=$((page + 1))
		[ "$page" -gt 20 ] && die "unexpectedly many pages; refusing to loop"
	done
}

main() {
	local floor=${1:-}
	local versions
	# Published CLI releases only: drop drafts, prereleases and non-CLI tags.
	versions=$(fetch | python3 -c '
import json, sys, re
for chunk in sys.stdin.read().split("\n["):
    text = chunk if chunk.startswith("[") else "[" + chunk
    try:
        items = json.loads(text)
    except json.JSONDecodeError:
        continue
    for r in items:
        if r.get("draft") or r.get("prerelease"):
            continue
        m = re.fullmatch(r"kustomize/v(\d+\.\d+\.\d+)", r.get("tag_name", ""))
        if m:
            print(m.group(1))
') || die "cannot parse the releases response"

	[ -n "$versions" ] || die "no kustomize CLI releases found"

	if [ -n "$floor" ]; then
		versions=$(printf '%s\n' "$versions" "${floor#v}" | sort -V -u |
			sed -n "/^${floor#v}\$/,\$p")
	fi
	printf '%s\n' "$versions" | sort -V -u
}

main "$@"
