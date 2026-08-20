# kustofmt -- every check CI runs is a target here, so nothing in the pipeline
# is unreproducible on a laptop. `make ci` is the whole pipeline.

BIN            := kustofmt
PKG            := ./...
LOCALBIN       := $(CURDIR)/bin
GOLANGCI_VER   := v2.13.1
GOLANGCI       := $(LOCALBIN)/golangci-lint
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
ci: fmt-check vet tidy-check lint test selfhost ## Everything CI runs

.PHONY: release-check
release-check: ci fuzz ## The fuller pre-release gate

.PHONY: clean
clean: ## Remove build and test artifacts
	rm -rf $(BIN) dist $(COVER) cover.html $(LOCALBIN)

$(GOLANGCI): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VER)

$(LOCALBIN):
	mkdir -p $(LOCALBIN)
