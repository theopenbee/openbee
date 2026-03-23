BINARY  := openbee
OUTDIR  := dist
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

.PHONY: help web build release clean npm-prepare npm-publish

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

web: ## Build frontend assets
	cd web && pnpm install --frozen-lockfile && pnpm build

build: ## Build binary for the current platform
	@mkdir -p $(OUTDIR)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(BINARY) ./cmd/openbee/

release: web ## Build binaries for all platforms
	@mkdir -p $(OUTDIR)
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		out=$(OUTDIR)/$(BINARY)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then out=$$out.exe; fi; \
		echo "  Building $$out..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o $$out ./cmd/openbee/ || exit 1; \
	done
	@echo "Done. Artifacts in $(OUTDIR)/"

clean: ## Remove build artifacts
	rm -rf $(OUTDIR)

npm-prepare:  ## Copy extracted dist/ binaries into npm package dirs and set version
	@VERSION=$(shell git describe --tags --always --dirty | sed 's/^v//'); \
	mkdir -p \
	    npm/packages/cli-linux-x64/bin \
	    npm/packages/cli-linux-arm64/bin \
	    npm/packages/cli-darwin-x64/bin \
	    npm/packages/cli-darwin-arm64/bin \
	    npm/packages/cli-win32-x64/bin \
	    npm/packages/cli-win32-arm64/bin; \
	cp $$(find dist -path '*/openbee*linux*amd64*' -name 'openbee') \
	    npm/packages/cli-linux-x64/bin/openbee; \
	cp $$(find dist -path '*/openbee*linux*arm64*' -name 'openbee') \
	    npm/packages/cli-linux-arm64/bin/openbee; \
	cp $$(find dist -path '*/openbee*darwin*amd64*' -name 'openbee') \
	    npm/packages/cli-darwin-x64/bin/openbee; \
	cp $$(find dist -path '*/openbee*darwin*arm64*' -name 'openbee') \
	    npm/packages/cli-darwin-arm64/bin/openbee; \
	cp $$(find dist -path '*/openbee*windows*amd64*' -name 'openbee.exe') \
	    npm/packages/cli-win32-x64/bin/openbee.exe; \
	cp $$(find dist -path '*/openbee*windows*arm64*' -name 'openbee.exe') \
	    npm/packages/cli-win32-arm64/bin/openbee.exe; \
	chmod +x \
	    npm/packages/cli-linux-x64/bin/openbee \
	    npm/packages/cli-linux-arm64/bin/openbee \
	    npm/packages/cli-darwin-x64/bin/openbee \
	    npm/packages/cli-darwin-arm64/bin/openbee; \
	node npm/scripts/set-version.js $$VERSION; \
	cp README.md npm/packages/cli/README.md

npm-publish: npm-prepare  ## Publish all npm packages to registry
	bash npm/scripts/publish.sh
