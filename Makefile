.PHONY: build build-ui run test clean install dist rpm lint lint-strict vet docker-up docker-stop

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -X main.version=$(VERSION)
BINARY = routatic-proxy
LEGACY_BINARY = oc-go-cc
CMD = ./cmd/routatic-proxy

# ── Development ────────────────────────────────────────────────────

build: build-css
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)
	@ln -sf $(BINARY) bin/$(LEGACY_BINARY)

build-ui: build-css
	CGO_ENABLED=1 go build -tags darwin -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(CMD)
	@ln -sf $(BINARY) bin/$(LEGACY_BINARY)

build-css:
	@echo "Building Tailwind CSS..."
	@npx tailwindcss -i internal/gui/assets/tailwind-input.css -o internal/gui/assets/compiled-tailwind.css --minify 2>&1 | grep -v "Browserslist:"

dmg: build-ui
	@./scripts/build_dmg.sh "$(VERSION)"

run:
	go run -ldflags "$(LDFLAGS)" $(CMD)

test:
	go test ./... -v -race

vet:
	go vet ./...

GOBIN=$(shell go env GOPATH)/bin

lint:
	@echo "Running gofmt..."
	@test -z "$$(gofmt -d . | tee /dev/stderr)" || (echo "gofmt check failed" && exit 1)
	@echo "Running go vet..."
	CGO_ENABLED=0 go vet ./...
	@echo "Lint checks passed!"

# Full lint pass with .golangci.yml — same command the pre-push hook runs.
GOLANGCI_LINT_VERSION = v2.13.1

lint-strict:
	@command -v golangci-lint >/dev/null || { \
		echo "golangci-lint not found. Install with:"; \
		echo "  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)"; \
		echo "  (or: brew install golangci-lint / dnf install golangci-lint)"; \
		exit 1; \
	}
	@echo "Running golangci-lint..."
	golangci-lint run --timeout 5m ./...

clean:
	rm -rf bin/ dist/

install: build
	@mkdir -p $(GOBIN)
	cp bin/$(BINARY) $(GOBIN)/$(BINARY)
	ln -sf $(BINARY) $(GOBIN)/$(LEGACY_BINARY)

# ── Docker ─────────────────────────────────────────────────────────

docker-up:
	@echo "Building Docker image..."
	docker build -t routatic-proxy .
	@echo ""
	@echo "Starting container..."
	@if [ ! -f .env ]; then \
		echo "ERROR: .env file not found."; \
		echo "Create it with: cp .env.example .env"; \
		exit 1; \
	fi
	@docker stop routatic-proxy 2>/dev/null || true
	@docker rm routatic-proxy 2>/dev/null || true
	docker run -d \
			--name routatic-proxy \
			--restart unless-stopped \
			--env-file .env \
			-p 3456:3456 \
			routatic-proxy
	@echo ""
	@echo "Container started! Proxy listening on http://localhost:3456"
	@echo "Stop with:  make docker-stop"

docker-stop:
	@echo "Stopping container..."
	docker stop routatic-proxy 2>/dev/null || true
	docker rm routatic-proxy 2>/dev/null || true
	@echo "Container stopped and removed."

# ── Release / Cross-Compilation ────────────────────────────────────

PLATFORMS = \
	darwin-amd64 \
	darwin-arm64 \
	linux-amd64 \
	linux-arm64 \
	windows-amd64 \
	windows-arm64

RELEASE_LDFLAGS = $(LDFLAGS) -s -w

dist: clean
	@mkdir -p dist
	@echo "Building release binaries (version: $(VERSION))..."
	@for platform in $(PLATFORMS); do \
		IFS='-' read -r GOOS GOARCH <<< "$$platform"; \
		EXT=""; \
		[ "$$GOOS" = "windows" ] && EXT=".exe"; \
		echo "  → $$GOOS/$$GOARCH"; \
		CGO_ENABLED=0 GOOS=$$GOOS GOARCH=$$GOARCH \
			go build -ldflags "$(RELEASE_LDFLAGS)" \
				-o "dist/$(BINARY)_$${platform}$${EXT}" \
				$(CMD); \
	done
	@echo ""
	@echo "Generating checksums..."
	@cd dist && sha256sum $(BINARY)_* > checksums.txt
	@echo ""
	@echo "Built binaries:"
	@ls -lh dist/

# ── RPM Packaging (Fedora / RHEL) ──────────────────────────────────
# Mirrors the CI job. Requires nfpm:
#   go install github.com/goreleaser/nfpm/v2/cmd/nfpm@$(NFPM_VERSION)
NFPM_VERSION = v2.47.0
RPM_VERSION ?= $(patsubst v%,%,$(VERSION))

rpm:
	@command -v nfpm >/dev/null || { \
		echo "nfpm not found. Install with:"; \
		echo "  go install github.com/goreleaser/nfpm/v2/cmd/nfpm@$(NFPM_VERSION)"; \
		exit 1; \
	}
	@mkdir -p dist
	@for arch in amd64 arm64; do \
		echo "  → linux/$$arch"; \
		CGO_ENABLED=0 GOOS=linux GOARCH=$$arch \
			go build -ldflags "$(RELEASE_LDFLAGS)" \
				-o "dist/$(BINARY)_linux-$$arch" $(CMD); \
		NFPM_VERSION="$(RPM_VERSION)" \
		NFPM_ARCH="$$arch" \
		NFPM_BINARY="dist/$(BINARY)_linux-$$arch" \
			nfpm package --config packaging/nfpm.yaml --packager rpm --target dist/; \
	done
	@echo ""
	@ls -lh dist/*.rpm
