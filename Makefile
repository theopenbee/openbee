BINARY  := robobee
OUTDIR  := dist
LDFLAGS := -s -w

PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

.PHONY: help web build release clean

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

web: ## Build frontend assets
	cd web && pnpm install --frozen-lockfile && pnpm build

build: ## Build binary for the current platform
	@mkdir -p $(OUTDIR)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(OUTDIR)/$(BINARY) ./cmd/robobee/

release: web ## Build binaries for all platforms
	@mkdir -p $(OUTDIR)
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d/ -f1); \
		arch=$$(echo $$platform | cut -d/ -f2); \
		out=$(OUTDIR)/$(BINARY)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then out=$$out.exe; fi; \
		echo "  Building $$out..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o $$out ./cmd/robobee/ || exit 1; \
	done
	@echo "Done. Artifacts in $(OUTDIR)/"

clean: ## Remove build artifacts
	rm -rf $(OUTDIR)
