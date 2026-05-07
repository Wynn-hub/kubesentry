export GOROOT := /opt/homebrew/Cellar/go/1.26.2/libexec

.PHONY: build build-linux image image-push helm-package helm-push release login clean test lint help

# ── Variables ─────────────────────────────────────────────────────────────────
REGISTRY ?= wynnhub
PLATFORMS ?= linux/amd64,linux/arm64
BUILDER   ?= kubesentry-builder
CHART_DIR  = charts/kubesentry
DIST_DIR   = dist

# VERSION is the single source of truth for all artifact naming.
# Override at the command line: make release VERSION=v0.1.0
#                               make release VERSION=test
# Defaults to the git describe output when not set.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# TAG: used as the Docker image tag, accepts any string (v0.1.0, test, v1, …).
TAG = $(VERSION)

# CHART_VERSION: Helm requires strict semver.
#   v0.1.0  → 0.1.0   (strip leading v, already semver)
#   v1      → 0.0.0-v1 (not semver, wrap as pre-release)
#   test    → 0.0.0-test
CHART_VERSION = $(shell \
  v="$(VERSION)"; semver="$${v\#v}"; \
  echo "$$semver" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]' \
    && echo "$$semver" \
    || echo "0.0.0-$$v")

WEBHOOK_IMAGE  = $(REGISTRY)/kubesentry-webhook:$(TAG)
OPERATOR_IMAGE = $(REGISTRY)/kubesentry-operator:$(TAG)

CMDS = webhook operator

# ── Step 1: Build (local native arch, for development) ───────────────────────
build:
	@mkdir -p bin
	go build -trimpath -o bin/webhook  ./cmd/webhook
	go build -trimpath -o bin/operator ./cmd/operator

# ── Step 2: Cross-compile inside a Docker container ──────────────────────────
# Produces: bin/linux-amd64/{webhook,operator}  bin/linux-arm64/{webhook,operator}
# Uses a Go container so the build env is isolated from the local toolchain.
GO_IMAGE ?= golang:1.26-alpine

build-linux:
	@mkdir -p $(HOME)/go/pkg/mod
	@for arch in amd64 arm64; do \
	  echo "→ building linux/$$arch in container"; \
	  mkdir -p bin/linux-$$arch; \
	  docker run --rm \
	    -v "$(CURDIR)":/app \
	    -v "$(HOME)/go/pkg/mod":/go/pkg/mod \
	    -w /app \
	    -e CGO_ENABLED=0 \
	    -e GOOS=linux \
	    -e GOARCH=$$arch \
	    $(GO_IMAGE) \
	    sh -c 'go build -trimpath -o bin/linux-'"$$arch"'/webhook ./cmd/webhook && \
	           go build -trimpath -o bin/linux-'"$$arch"'/operator ./cmd/operator' \
	  || exit 1; \
	done

# ── Test & Lint ───────────────────────────────────────────────────────────────
test:
	go test ./... -count=1

lint:
	go vet ./...

# ── Step 3: Package into multi-platform images ────────────────────────────────
# buildx reads bin/linux-{amd64,arm64}/ via TARGETARCH injected at build time.
.builder:
	docker buildx inspect $(BUILDER) >/dev/null 2>&1 || \
	  docker buildx create --name $(BUILDER) --driver docker-container --bootstrap
	@touch .builder

# Load into local Docker daemon (requires containerd image store).
image: .builder build-linux
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.webhook \
	  --tag $(WEBHOOK_IMAGE) \
	  --load \
	  .
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.operator \
	  --tag $(OPERATOR_IMAGE) \
	  --load \
	  .

# Build and push multi-platform manifest to registry.
image-push: .builder build-linux
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.webhook \
	  --tag $(WEBHOOK_IMAGE) \
	  --push \
	  .
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.operator \
	  --tag $(OPERATOR_IMAGE) \
	  --push \
	  .

# ── Step 4: Package Helm chart ────────────────────────────────────────────────
helm-package:
	@mkdir -p $(DIST_DIR)
	helm lint $(CHART_DIR)
	helm package $(CHART_DIR) \
	  --version $(CHART_VERSION) \
	  --app-version $(TAG) \
	  --destination $(DIST_DIR)
	@echo "Chart: $(DIST_DIR)/kubesentry-$(CHART_VERSION).tgz"

# Push the packaged chart to Docker Hub as an OCI artifact.
# Requires prior login: helm registry login registry-1.docker.io -u $(REGISTRY)
helm-push: helm-package
	helm push $(DIST_DIR)/kubesentry-$(CHART_VERSION).tgz \
	  oci://registry-1.docker.io/$(REGISTRY)
	@echo "Chart pushed: oci://registry-1.docker.io/$(REGISTRY)/kubesentry:$(CHART_VERSION)"

# ── Full release pipeline ─────────────────────────────────────────────────────
release: test image-push helm-push
	@echo ""
	@echo "Release complete:"
	@echo "  webhook:  $(WEBHOOK_IMAGE)"
	@echo "  operator: $(OPERATOR_IMAGE)"
	@echo "  chart:    oci://registry-1.docker.io/$(REGISTRY)/kubesentry:$(CHART_VERSION)"

# ── Login ─────────────────────────────────────────────────────────────────────
# Login to Docker Hub for both image push and helm chart push.
# Run once before the first `make release` or whenever credentials expire.
login:
	docker login -u $(REGISTRY)
	helm registry login registry-1.docker.io -u $(REGISTRY)

# ── Clean ─────────────────────────────────────────────────────────────────────
clean:
	rm -rf bin/ $(DIST_DIR)/ .builder
	docker buildx rm $(BUILDER) 2>/dev/null || true

# ── Help ──────────────────────────────────────────────────────────────────────
help:
	@echo "Usage: make <target> [VERSION=<version>] [REGISTRY=<registry>]"
	@echo ""
	@echo "Variables:"
	@printf "  %-20s %s\n" "VERSION"  "Artifact version (default: git describe). e.g. v0.1.0, v1, test"
	@printf "  %-20s %s\n" "REGISTRY" "Image registry prefix (default: wynnhub)"
	@printf "  %-20s %s\n" "PLATFORMS" "Target platforms (default: linux/amd64,linux/arm64)"
	@printf "  %-20s %s\n" "GO_IMAGE" "Go builder image (default: golang:1.26-alpine)"
	@echo ""
	@echo "Development:"
	@printf "  %-20s %s\n" "build"        "Compile for local arch → bin/webhook, bin/operator"
	@printf "  %-20s %s\n" "test"         "Run all tests"
	@printf "  %-20s %s\n" "lint"         "Run go vet"
	@echo ""
	@echo "Release:"
	@printf "  %-20s %s\n" "build-linux"  "Cross-compile in Docker → bin/linux-amd64/, bin/linux-arm64/"
	@printf "  %-20s %s\n" "image"        "Build multi-platform images and load into local daemon"
	@printf "  %-20s %s\n" "image-push"   "Build and push multi-platform images to registry"
	@printf "  %-20s %s\n" "helm-package" "Lint and package Helm chart → dist/kubesentry-<version>.tgz"
	@printf "  %-20s %s\n" "helm-push"    "Push Helm chart to oci://registry-1.docker.io/\$$REGISTRY"
	@printf "  %-20s %s\n" "release"      "Full pipeline: test → image-push → helm-push"
	@echo ""
	@echo "Maintenance:"
	@printf "  %-20s %s\n" "login"        "Login to Docker Hub (run once before release)"
	@printf "  %-20s %s\n" "clean"        "Remove bin/, dist/, buildx builder"
	@echo ""
	@echo "Examples:"
	@printf "  %s\n" "  make release VERSION=v0.1.0"
	@printf "  %s\n" "  make release VERSION=test"
	@printf "  %s\n" "  make image VERSION=v1 REGISTRY=myrepo"
	@printf "  %s\n" "  make helm-package VERSION=v0.2.0"
