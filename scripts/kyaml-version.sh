#!/usr/bin/env bash
# Print the kyaml version this module builds against.
#
# The output style is kyaml's emission, so this is the version that defines the
# style contract. Release notes record it; `kustofmt -version` reports it.
set -euo pipefail
cd "$(dirname "$0")/.."
go list -m -f '{{.Version}}' sigs.k8s.io/kustomize/kyaml
