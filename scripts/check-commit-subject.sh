#!/usr/bin/env bash
# Validate one commit subject against kustofmt's convention.
#
# The convention is the Go project's -- "package: description" -- and not
# Conventional Commits. That is deliberate. This tool's whole thesis is that a
# style should be adopted from its ecosystem rather than invented, and for a Go
# project the Go style is the ecosystem's. CONTRIBUTING.md records the longer
# reasoning, including why release-please does not fit.
#
# This script is the only place the rule is written down. Three callers share
# it: the commit-msg hook, `make commit-check`, and the CI lint job. A rule
# stated twice is a rule that drifts.
#
# Usage:
#   check-commit-subject.sh <message-file>   # git commit-msg hook form
#   check-commit-subject.sh -                # read one subject from stdin
set -euo pipefail

# The allowed prefixes: a package path for code, a Go-style category otherwise.
# "compat" covers cmd/compat and internal/compat; "release" covers
# internal/release, .goreleaser.yaml and release.yaml.
#
# "deps" and "ci" are also exactly what Dependabot is configured to emit (see
# .github/dependabot.yml), so bot commits need no special case here. Only their
# length does -- see below.
PREFIXES=(
	format
	cmd/kustofmt
	compat
	release
	docs
	ci
	build
	test
	deps
)

readonly MAX_SUBJECT=72

# Subjects git or a tool generates, which the author does not control.
readonly EXEMPT='^(Merge |Revert "|fixup!|squash!)'

# Dependabot's grouped updates name the group and the count -- "bump the
# go-dependencies group across 1 directory with 12 updates" -- and legitimately
# run past the limit. The prefix is still checked; only the length is waived.
readonly LONG_ALLOWED='^(deps|ci): bump '

die() {
	local subject=$1 reason=$2
	{
		echo "commit subject rejected: ${reason}"
		echo
		echo "    ${subject}"
		echo
		echo "Subjects are \"prefix: description\", where prefix is one of:"
		echo
		printf '    %s\n' "${PREFIXES[*]}"
		echo
		echo "and description is lowercase and imperative, with no trailing period."
		echo "The whole subject must be at most ${MAX_SUBJECT} characters."
		echo
		echo "For example:"
		echo "    format: preserve the leading document separator"
		echo "    cmd/kustofmt: answer -h on stdout and exit zero"
		echo "    docs: describe how kustomize releases are tracked"
	} >&2
	exit 1
}

# join_prefixes renders the allowlist as an alternation for the match below.
join_prefixes() {
	local IFS='|'
	printf '%s' "${PREFIXES[*]}"
}

main() {
	local src=${1:-}
	if [ -z "$src" ]; then
		echo "usage: $0 <message-file>|-" >&2
		exit 2
	fi

	local message
	if [ "$src" = - ]; then
		message=$(cat)
	elif [ -f "$src" ]; then
		message=$(cat "$src")
	else
		echo "$0: no such message file: $src" >&2
		exit 2
	fi

	# The subject is the first line that is neither blank nor a comment. git
	# leaves its own comment block in the file the commit-msg hook receives, and
	# strips it afterwards, so skipping those lines is not optional.
	local subject
	subject=$(printf '%s\n' "$message" | awk '/^#/ {next} /[^[:space:]]/ {print; exit}')

	# Nothing to judge. An empty message is git's business: it aborts the commit
	# on its own, and failing here would report the wrong reason.
	[ -n "$subject" ] || exit 0

	[[ $subject =~ $EXEMPT ]] && exit 0

	# Declared separately: assigning in the declaration would mask the exit
	# status of the substitution (shellcheck SC2155).
	local pattern
	pattern="^($(join_prefixes)): "
	[[ $subject =~ $pattern ]] || die "$subject" "no recognised prefix"

	local description=${subject#*: }
	[ -n "$description" ] || die "$subject" "no description after the prefix"
	[[ $description =~ ^[A-Z] ]] && die "$subject" "description starts with a capital"
	[[ $description == *. ]] && die "$subject" "description ends with a period"

	if [ ${#subject} -gt $MAX_SUBJECT ] && ! [[ $subject =~ $LONG_ALLOWED ]]; then
		die "$subject" "${#subject} characters, over the ${MAX_SUBJECT} limit"
	fi
	exit 0
}

main "$@"
