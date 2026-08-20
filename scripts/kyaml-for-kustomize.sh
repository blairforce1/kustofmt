#!/usr/bin/env bash
# Resolve which kyaml a given kustomize CLI release links.
#
#   scripts/kyaml-for-kustomize.sh 5.7.1   ->  v0.20.1
#
# kustofmt's output style is kyaml's emitter, so this mapping is the whole basis
# of the compatibility matrix. It has to be derived, never guessed:
#
#   kustomize v5.4.1 ships api v0.17.1 but kyaml v0.17.0
#   kustomize v5.5.0 ships api v0.18.0 but kyaml v0.18.1  (kyaml AHEAD of api)
#
# so the tidy-looking "api v0.M.P <-> CLI v5.(M-13).P" relation -- which is sound
# for deriving the CLI from the api version -- is wrong for kyaml in four of the
# ten v5 releases checked. Only the dependency graph is the truth.
#
# Two independent resolutions must agree:
#
#   MVS     what the Go toolchain actually selects for that module version,
#           honouring transitive requirements and replace directives. This is
#           definitionally what the released binary links.
#   go.mod  what upstream declared, read straight from the tag.
#
# They agree for every release checked. A disagreement means upstream has done
# something the matrix does not model, so this exits non-zero rather than
# picking a winner. Stopping and fetching a human beats a confident wrong answer.
set -euo pipefail

readonly MODULE=sigs.k8s.io/kustomize/kustomize/v5
readonly KYAML=sigs.k8s.io/kustomize/kyaml
readonly RAW=https://raw.githubusercontent.com/kubernetes-sigs/kustomize

usage() {
	cat >&2 <<-EOF
		usage: $(basename "$0") <kustomize-version>

		  <kustomize-version>  e.g. 5.7.1 or v5.7.1

		Prints the kyaml version that kustomize release links, e.g. v0.20.1.

		Environment:
		  KUSTOFMT_NO_CACHE=1   bypass the on-disk cache
	EOF
}

die() {
	echo "kyaml-for-kustomize: $*" >&2
	exit 2
}

# cache_path echoes where the answer for a version is cached. Released tags are
# immutable, so a hit is never stale.
cache_path() {
	local dir="${XDG_CACHE_HOME:-$HOME/.cache}/kustofmt/kyaml-for-kustomize"
	mkdir -p "$dir" 2>/dev/null || return 1
	echo "$dir/$1"
}

# resolve_mvs asks the Go toolchain what it selects. A throwaway module keeps
# this from touching the repository's own go.mod.
resolve_mvs() {
	local version=$1 dir
	dir=$(mktemp -d) || die "cannot create a temporary directory"
	# shellcheck disable=SC2064 # expand dir now, not at trap time
	trap "rm -rf '$dir'" RETURN
	printf 'module kustofmt-probe\n\ngo 1.26\n' >"$dir/go.mod"
	(
		cd "$dir"
		GOFLAGS=-mod=mod go get "$MODULE@$version" >/dev/null 2>&1
	) || return 1
	(cd "$dir" && go list -m -f '{{.Version}}' "$KYAML" 2>/dev/null)
}

# resolve_gomod reads the dependency upstream declared, preferring a replace
# directive over the require line: a replace is what would actually be built.
resolve_gomod() {
	local version=$1 mod
	mod=$(curl -fsSL --max-time 30 "$RAW/kustomize/$version/kustomize/go.mod" 2>/dev/null) || return 1

	local replaced
	replaced=$(printf '%s\n' "$mod" |
		grep -E "^[[:space:]]*(replace[[:space:]]+)?${KYAML}[[:space:]]+.*=>" |
		grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+[^[:space:]]*$' | head -1) || true
	if [ -n "$replaced" ]; then
		printf '%s\n' "$replaced"
		return 0
	fi

	printf '%s\n' "$mod" |
		grep -E "^[[:space:]]*${KYAML} v" |
		grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+[^[:space:]]*' | head -1
}

main() {
	[ $# -eq 1 ] || { usage; exit 2; }
	case $1 in
	-h | --help) usage; exit 0 ;;
	esac

	# Accept 5.7.1 or v5.7.1; the tag carries the v.
	local version=v${1#v}
	[[ $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]] || die "not a version: $1"

	local cache=""
	if [ "${KUSTOFMT_NO_CACHE:-}" != "1" ] && cache=$(cache_path "$version") && [ -s "$cache" ]; then
		cat "$cache"
		return 0
	fi

	local mvs gomod
	mvs=$(resolve_mvs "$version") ||
		die "cannot resolve $MODULE@$version through the Go toolchain (unknown version, or no network)"
	gomod=$(resolve_gomod "$version") ||
		die "cannot fetch kustomize/go.mod at tag kustomize/$version (unknown version, or no network)"

	[ -n "$mvs" ] || die "the Go toolchain reported no $KYAML for $version"
	[ -n "$gomod" ] || die "kustomize/go.mod at $version declares no $KYAML"

	if [ "$mvs" != "$gomod" ]; then
		die "$(
			cat <<-EOF
				the two resolutions disagree for kustomize $version:
				  MVS selects      $mvs
				  go.mod declares  $gomod
				Upstream has done something this matrix does not model -- a transitive
				bump, or a replace directive. Resolve it by hand before trusting either.
			EOF
		)"
	fi

	if [ -n "$cache" ]; then
		# A cache miss is not worth failing over: the answer is already known.
		printf '%s\n' "$mvs" >"$cache" 2>/dev/null || true
	fi
	printf '%s\n' "$mvs"
}

main "$@"
