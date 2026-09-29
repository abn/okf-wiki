.DEFAULT_GOAL := help

IMAGE   ?= ghcr.io/abn/okf-wiki:latest
CONTENT ?= docs
OUT     ?= .scratch/wiki
PORT    ?= 8080
BASE    ?= /wiki/
# THEME is a theme directory layered over the embedded default. Leave it empty to
# use the theme compiled into the binary.
THEME   ?=
BRAND   ?= okf-wiki
SUB     ?= docs
TITLE   ?= Wiki

# $(THEME) is only passed when set, so an empty THEME does not become a
# --theme "" that would resolve the wrong directory.
THEME_FLAG = $(if $(THEME),--theme $(THEME))
BRANDING   = --brand $(BRAND) --brand-sub $(SUB) --title $(TITLE) --base $(BASE) $(THEME_FLAG)

# Keep the Go build cache inside the workspace (sandbox-friendly).
export GOCACHE ?= $(CURDIR)/.scratch/gocache

.PHONY: setup vendor build test vet fmt lint check run render render-themed \
        container/build container/run container/shell clean help

##@ Bootstrap

setup: ## Download Go modules
	go mod download

vendor: ## Build the offline mermaid+ELK bundle (needs node/npm)
	cd mermaid && npm ci --no-audit --no-fund --legacy-peer-deps
	cd mermaid && npx esbuild entry.js --bundle --format=esm --minify --outfile=mermaid-bundle.min.mjs
	@printf '\n  mermaid/mermaid-bundle.min.mjs ready (%s)\n\n' "$$(du -h mermaid/mermaid-bundle.min.mjs | cut -f1)"

##@ Development

run: vendor ## Render $(CONTENT) and serve on 0.0.0.0:$(PORT), opening a browser
	go run ./cmd/okf-wiki serve --content $(CONTENT) --out $(OUT) --addr 0.0.0.0:$(PORT) --open \
	  $(BRANDING)

render: ## Render $(CONTENT) into $(OUT)
	go run ./cmd/okf-wiki render --content $(CONTENT) --out $(OUT) $(BRANDING)

# Renders the same bundle with a theme directory mounted in, which is the
# workflow the container uses. THEME=path/to/theme make render-themed
render-themed: ## Render $(CONTENT) with THEME=DIR layered over the embedded theme
	@test -n "$(THEME)" || { echo 'usage: make render-themed THEME=DIR'; exit 2; }
	@$(MAKE) --no-print-directory render THEME=$(THEME)

##@ Build & Quality

build: ## Build the okf-wiki binary into bin/
	go build -o bin/okf-wiki ./cmd/okf-wiki

test: ## Run the test suite
	go test ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format Go sources
	gofmt -w cmd internal

lint: fmt vet ## Format, then vet

check: lint test ## Full gate: format, vet, tests

clean: ## Remove build output and the vendor bundle
	rm -rf bin $(OUT) mermaid/node_modules mermaid/mermaid-bundle.min.mjs

##@ Container

container/build: ## Build the container image ($(IMAGE))
	podman build --format docker -f Containerfile -t $(IMAGE) .

container/run: ## Run the image against ./docs (host network, auto-detected content)
	podman run --rm --network=host -v $(CURDIR)/$(CONTENT):$(CURDIR)/$(CONTENT):ro,Z $(IMAGE)

container/shell: ## Open a shell in the built image
	podman run --rm -it --entrypoint /bin/sh $(IMAGE)

##@ Utilities

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} \
	  /^[a-zA-Z0-9_/-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } \
	  /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
