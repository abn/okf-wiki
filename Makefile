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
# SECTIONS renames or orders a directory in the sidebar without changing its
# URL. The adr/ directory is a section id; it reads as Decisions.
SECTIONS ?= adr:Decisions

# $(THEME) is only passed when set, so an empty THEME does not become a
# --theme "" that would resolve the wrong directory.
THEME_FLAG    = $(if $(THEME),--theme $(THEME))
SECTIONS_FLAG = $(if $(SECTIONS),--sections "$(SECTIONS)")

# The project's own wiki opts in to the source archive, which is off by default
# because it puts a second copy of the bundle in the output.
BRANDING   = --brand $(BRAND) --brand-sub $(SUB) --title $(TITLE) --base $(BASE) \
             --bundle $(THEME_FLAG) $(SECTIONS_FLAG)

# Keep the Go build cache inside the workspace (sandbox-friendly).
export GOCACHE ?= $(CURDIR)/.scratch/gocache

.PHONY: setup vendor build test vet fmt lint check run render render-themed firefox \
        screenshots social-card container/build container/run container/shell clean help

##@ Bootstrap

setup: ## Download Go modules
	go mod download

vendor: ## Build the offline mermaid+ELK bundles (needs node/npm)
	cd mermaid && npm ci --no-audit --no-fund --legacy-peer-deps
	cd mermaid && npx esbuild entry.js --bundle --format=esm --minify --outfile=mermaid-bundle.min.mjs
	cd mermaid && npx esbuild entry-iife.js --bundle --format=iife --minify --outfile=mermaid-bundle.min.js
	@printf '\n  mermaid/mermaid-bundle.min.mjs ready (%s)\n' "$$(du -h mermaid/mermaid-bundle.min.mjs | cut -f1)"
	@printf '  mermaid/mermaid-bundle.min.js ready (%s)\n\n' "$$(du -h mermaid/mermaid-bundle.min.js | cut -f1)"

##@ Development

# ADDR is loopback by default: the wiki has no authentication, so a local dev
# server should not be reachable from the network unless that is asked for.
# ADDR=0.0.0.0 make run publishes it to the LAN.
ADDR   ?= 127.0.0.1:$(PORT)

run: vendor ## Render $(CONTENT) and serve on $(ADDR), opening a browser
	go run ./cmd/okf-wiki serve --content $(CONTENT) --out $(OUT) --addr $(ADDR) --open \
	  $(BRANDING)

render: ## Render $(CONTENT) into $(OUT)
	go run ./cmd/okf-wiki render --content $(CONTENT) --out $(OUT) $(BRANDING)

# Renders the same bundle with a theme directory mounted in, which is the
# workflow the container uses. THEME=path/to/theme make render-themed
render-themed: ## Render $(CONTENT) with THEME=DIR layered over the embedded theme
	@test -n "$(THEME)" || { echo 'usage: make render-themed THEME=DIR'; exit 2; }
	@$(MAKE) --no-print-directory render THEME=$(THEME)

firefox: ## Render $(CONTENT) offline and run the Firefox smoke test (needs Firefox and Selenium)
	@$(MAKE) --no-print-directory vendor
	go run ./cmd/okf-wiki render --content $(CONTENT) --out .scratch/offline --offline $(BRANDING)
	python3 scripts/check-offline-firefox.py .scratch/offline/wiki

# The README images are regenerated from the same bundle the site uses, so a
# change to the theme or the renderer shows up in the next capture.
screenshots: vendor ## Capture the README screenshots (needs Playwright and Chrome)
	python3 scripts/capture-readme-screenshots.py

# The cards are static and checked in, so this runs only when the card, the
# brand mark, or the theme's tokens or fonts change. GitHub takes one image at
# 1280x640; the light and dark pair is kept so a re-skin has both ready.
social-card: ## Render the GitHub social preview cards (needs Chrome)
	python3 scripts/capture-social-card.py

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
	rm -rf bin $(OUT) mermaid/node_modules mermaid/mermaid-bundle.min.mjs mermaid/mermaid-bundle.min.js

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
