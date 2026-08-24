# kustofmt -- every check CI runs is a target here, so nothing in the pipeline
# is unreproducible on a laptop. `make ci` is the whole pipeline.

BIN            := kustofmt
PKG            := ./...
LOCALBIN       := $(CURDIR)/bin
GOLANGCI_VER   := v2.13.1
GOLANGCI       := $(LOCALBIN)/golangci-lint
# Pinned: an unpinned linter means local and CI disagree about what is clean,
# and the disagreement only shows up as a red build after a tag is cut.
SHELLCHECK_IMAGE := docker.io/koalaman/shellcheck:v0.11.0
# Both globs. The commit-msg hook has no .sh extension -- git requires that
# exact name -- and an unlinted hook is how one quietly stops working.
SHELL_SOURCES  := scripts/*.sh scripts/hooks/*
# The ref a pull request is measured against. Override for a local check:
# `make commit-check COMMIT_BASE=HEAD~5`.
COMMIT_BASE    ?= origin/main
VERSION        ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
FUZZTIME       ?= 30s
COVER          := cover.out

# Reproducible, static, no local paths baked into the binary. CGO is disabled
# per-target rather than globally: the race detector requires cgo, so exporting
# CGO_ENABLED=0 for the whole file silently disables `make test`.
GOFLAGS_BUILD  := -trimpath
LDFLAGS        := -s -w -X main.version=$(VERSION)

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the static binary
	CGO_ENABLED=0 go build $(GOFLAGS_BUILD) -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/kustofmt

.PHONY: test-fast
test-fast: ## Unit, golden and CLI tests only (the PR lane)
	go test -short $(PKG)

.PHONY: test
test: ## Full test suite with the race detector and coverage
	go test -race -covermode=atomic -coverprofile=$(COVER) $(PKG)

.PHONY: cover
cover: test ## Show coverage per function
	go tool cover -func=$(COVER)

.PHONY: cover-html
cover-html: test ## Open an HTML coverage report
	go tool cover -html=$(COVER) -o cover.html
	@echo "wrote cover.html"

.PHONY: fuzz
fuzz: ## Run each fuzz target for FUZZTIME (default 30s)
	@for t in FuzzIdempotent FuzzSemanticsPreserved FuzzDiffApplies FuzzIsSOPSNeverPanics; do \
		echo "==> $$t"; \
		go test ./format/ -run "^$$t$$" -fuzz "^$$t$$" -fuzztime=$(FUZZTIME) || exit 1; \
	done

.PHONY: lint
lint: $(GOLANGCI) ## Run golangci-lint
	$(GOLANGCI) run

.PHONY: fmt-check
fmt-check: ## Fail if any Go file is not gofmt-clean
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	@echo "gofmt: clean"

.PHONY: vet
vet: ## Run go vet
	go vet $(PKG)

.PHONY: tidy-check
tidy-check: ## Fail if go.mod/go.sum are not tidy
	@cp go.mod go.mod.bak && cp go.sum go.sum.bak
	@go mod tidy
	@if ! diff -q go.mod go.mod.bak >/dev/null || ! diff -q go.sum go.sum.bak >/dev/null; then \
		mv go.mod.bak go.mod; mv go.sum.bak go.sum; \
		echo "go.mod/go.sum are not tidy; run 'go mod tidy'"; exit 1; \
	fi
	@rm -f go.mod.bak go.sum.bak
	@echo "modules: tidy"

.PHONY: compat-status
compat-status: ## Show kustomize releases not yet in the compatibility matrix
	go run ./cmd/compat status

.PHONY: compat-render
compat-render: ## Regenerate the compatibility tables in README and CHANGELOG
	go run ./cmd/compat render

.PHONY: compat-check
compat-check: ## Verify the matrix is correct: go.mod, docs, and every recorded row (needs network)
	go run ./cmd/compat check

.PHONY: compat-check-complete
compat-check-complete: ## As compat-check, plus: no published kustomize release is missing
	@# Only true on main. A tag cut before a kustomize release cannot know about
	@# it, so release builds run compat-check instead.
	go run ./cmd/compat check --complete

.PHONY: cosign-version
cosign-version: ## Print the cosign version the release workflow pins
	@# One source of truth for the pin: release.yaml. CI reads it from here so
	@# the job that checks the signing arguments installs the cosign the release
	@# actually uses -- a check run against a different cosign is not a check.
	@grep -oE 'cosign-release: v[^ ]+' .github/workflows/release.yaml | head -1 | cut -d' ' -f2

.PHONY: sign-check
sign-check: ## Run .goreleaser.yaml's cosign arguments and check they write what it declares
	@# `make test` covers this too; this target exists so the result is visible
	@# rather than folded into the suite, and so CI has something narrow to run
	@# in the one job that installs cosign. Skips unless the cosign on PATH is
	@# the pinned one.
	go test ./internal/release/ -run TestSigning -v

.PHONY: shellcheck
shellcheck: ## Lint the shell scripts (pinned; see SHELLCHECK_IMAGE)
	@runner=$$(command -v docker || command -v podman || true); \
	if [ -n "$$runner" ]; then \
		"$$runner" run --rm -v "$(CURDIR):/mnt:ro,z" -w /mnt \
			$(SHELLCHECK_IMAGE) $(SHELL_SOURCES) && echo "shellcheck: clean ($(SHELLCHECK_IMAGE))"; \
	elif command -v shellcheck >/dev/null 2>&1; then \
		echo "shellcheck: no container runtime; using the system binary, which may"; \
		echo "            differ from the pinned $(SHELLCHECK_IMAGE)"; \
		shellcheck $(SHELL_SOURCES) && echo "shellcheck: clean"; \
	else \
		echo "shellcheck: unavailable and no container runtime; skipped"; \
	fi

.PHONY: commit-check
commit-check: ## Check commit subjects on COMMIT_BASE..HEAD against the convention
	@# scripts/check-commit-subject.sh holds the rule; this only supplies the
	@# range. Skips rather than fails when the base ref is absent: a shallow
	@# clone has nothing to compare against, and a check that cannot run should
	@# say so. Passing quietly is how a gate stops being one.
	@if ! git rev-parse -q --verify '$(COMMIT_BASE)' >/dev/null 2>&1; then \
		echo "commit-check: $(COMMIT_BASE) is not available; skipped"; \
	else \
		bad=$$(git log --format='%s' '$(COMMIT_BASE)..HEAD' | while IFS= read -r s; do \
			printf '%s\n' "$$s" | ./scripts/check-commit-subject.sh - || printf 'x'; \
		done); \
		if [ -n "$$bad" ]; then exit 1; fi; \
		echo "commit-check: clean"; \
	fi

.PHONY: hooks
hooks: ## Install this repository's git hooks (sets core.hooksPath)
	@# Opt-in, and never the only gate -- CI runs commit-check over the whole
	@# pull request, because a fork has no hooks and --no-verify skips them.
	@git config core.hooksPath scripts/hooks
	@echo "hooks: core.hooksPath -> scripts/hooks"
	@echo "       this REPLACES .git/hooks, so any hook already there stops running"
	@echo "       undo with: git config --unset core.hooksPath"

.PHONY: selfhost
selfhost: build ## The formatter's own repository must pass its own formatter
	@# format/testdata is excluded by design: those files are deliberately not in
	@# house style, because they are the *inputs* that prove the transformation.
	@out=$$(git ls-files -z '*.yaml' '*.yml' | grep -zv '^format/testdata/' | \
		xargs -0 -r ./$(BIN) -l); \
	if [ -n "$$out" ]; then echo "these files need formatting:"; echo "$$out"; exit 1; fi; \
	echo "selfhost: clean"

.PHONY: golden
golden: ## Regenerate golden files after an intentional style change
	go test ./format/ -update
	@echo "goldens updated -- review the diff before committing: a change here is a breaking change"

.PHONY: ci
ci: fmt-check vet tidy-check lint shellcheck commit-check test selfhost ## Everything CI runs (offline)

.PHONY: release-check
release-check: ci fuzz compat-check ## The fuller pre-release gate (compat-check needs network)

.PHONY: clean
clean: ## Remove build and test artifacts
	rm -rf $(BIN) dist $(COVER) cover.html $(LOCALBIN)

$(GOLANGCI): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VER)

$(LOCALBIN):
	mkdir -p $(LOCALBIN)
