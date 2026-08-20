#!/usr/bin/env bash
# Regenerate the evidence behind the README's comparison table.
#
# The table makes specific claims about other people's software. Those claims
# have to be re-runnable, or they decay into folklore the moment upstream ships
# a release. Run this, read the output, update the table.
#
# Requires: kustofmt (make build), and whichever of yamlfmt/yq/prettier you have.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1
KUSTOFMT="${KUSTOFMT:-./kustofmt}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

have() { command -v "$1" >/dev/null 2>&1; }

echo "=== tool versions ==="
"$KUSTOFMT" -version 2>/dev/null | head -1 || echo "kustofmt: not built (run make build)"
have yamlfmt  && yamlfmt -version 2>&1 | head -1 | sed 's/^/yamlfmt /'
have yq       && yq --version
have prettier && echo "prettier $(prettier --version)"
have kustomize && kustomize version | sed 's/^/kustomize /'
echo

# yamlfmt configured as favourably as its options allow.
cat > "$WORK/.yamlfmt" <<'EOF'
formatter:
  type: basic
  indentless_arrays: true
  force_array_style: block
  retain_line_breaks: true
EOF

run_case() {
  local name="$1" input="$2"
  echo "############################################################"
  echo "# CASE: $name"
  echo "############################################################"
  printf '%s' "$input" > "$WORK/in.yaml"
  echo "--- input ---"; cat "$WORK/in.yaml"

  echo "--- kustofmt ---"
  "$KUSTOFMT" < "$WORK/in.yaml" 2>&1 || echo "(failed)"

  if have yamlfmt; then
    echo "--- yamlfmt (indentless_arrays + force_array_style: block) ---"
    cp "$WORK/in.yaml" "$WORK/y.yaml"
    (cd "$WORK" && yamlfmt -conf .yamlfmt y.yaml >/dev/null 2>&1)
    cat "$WORK/y.yaml"
  fi

  if have yq; then
    echo "--- yq (identity filter) ---"
    yq '.' "$WORK/in.yaml" 2>&1 || echo "(failed)"
  fi

  if have prettier; then
    echo "--- prettier ---"
    cp "$WORK/in.yaml" "$WORK/p.yaml"
    prettier --write "$WORK/p.yaml" >/dev/null 2>&1 && cat "$WORK/p.yaml" || echo "(failed)"
  fi
  echo
}

run_case "indentless sequences (the house style)" \
'spec:
  containers:
    - name: app
      ports:
        - containerPort: 8080
'

run_case "non-empty flow map must become block" \
'resources:
  requests: {cpu: 50m, memory: 64Mi}
  limits: {}
'

run_case "leading --- must round-trip per file (Flux export)" \
'---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: app
'

run_case "no leading --- must stay bare (kustomize output)" \
'apiVersion: v1
kind: ConfigMap
metadata:
  name: app
'

echo "############################################################"
echo "# CASE: sops-encrypted file must not be reformatted"
echo "############################################################"
if [ -f format/testdata/sops-secret.yaml ]; then
  cp format/testdata/sops-secret.yaml "$WORK/secret.yaml"
  echo "--- kustofmt ---"
  "$KUSTOFMT" -w "$WORK/secret.yaml" 2>&1
  if cmp -s format/testdata/sops-secret.yaml "$WORK/secret.yaml"; then
    echo "RESULT: unchanged (MAC intact)"
  else
    echo "RESULT: MODIFIED (MAC broken)"
  fi

  if have yamlfmt; then
    echo "--- yamlfmt ---"
    cp format/testdata/sops-secret.yaml "$WORK/s2.yaml"
    (cd "$WORK" && yamlfmt -conf .yamlfmt s2.yaml >/dev/null 2>&1)
    if cmp -s format/testdata/sops-secret.yaml "$WORK/s2.yaml"; then
      echo "RESULT: unchanged"
    else
      echo "RESULT: MODIFIED (MAC broken)"
    fi
  fi
fi
