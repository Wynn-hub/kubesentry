export GOROOT := /opt/homebrew/opt/go/libexec

.PHONY: build build-linux image image-push helm-package helm-push release login clean \
        test lint test-report test-e2e test-e2e-report test-all build-image-e2e tools help \
        build-image-local deploy-local undeploy-local web-build test-web dev-local \
        proto api-gen api-lint

# ── Variables ─────────────────────────────────────────────────────────────────
REGISTRY ?= wynnhub
PLATFORMS ?= linux/amd64,linux/arm64
BUILDER   ?= kubesentry-builder
CHART_DIR  = charts/kubesentry
GATEWAY_CHART_DIR = charts/kubesentry-gateway
DIST_DIR   = dist
REPORT_DIR = $(DIST_DIR)/test-reports

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

WEBHOOK_IMAGE     = $(REGISTRY)/kubesentry-webhook:$(TAG)
OPERATOR_IMAGE    = $(REGISTRY)/kubesentry-operator:$(TAG)
CONSOLE_IMAGE     = $(REGISTRY)/kubesentry-console:$(TAG)
GATEWAY_IMAGE     = $(REGISTRY)/kubesentry-gateway:$(TAG)
DB_IMAGE          = $(REGISTRY)/kubesentry-db:$(TAG)
CONSOLE_WEB_IMAGE = $(REGISTRY)/kubesentry-console-web:$(TAG)

CMDS = webhook operator console

# Test tooling — auto-installed to GOPATH/bin on first use.
GOTESTSUM = $(shell go env GOPATH)/bin/gotestsum

$(GOTESTSUM):
	go install gotest.tools/gotestsum@latest

# Install all test tooling in one shot.
tools: $(GOTESTSUM)

# ── Web console frontend ─────────────────────────────────────────────────────
web-build:
	cd web && npm ci && npm run build

test-web:
	cd web && npm ci && npm run test

# ── Step 1: Build (local native arch, for development) ───────────────────────
build:
	@mkdir -p bin
	go build -trimpath -o bin/webhook  ./cmd/webhook
	go build -trimpath -o bin/operator ./cmd/operator
	go build -trimpath -o bin/console  ./cmd/console
	go build -trimpath -o bin/gateway  ./cmd/gateway
	go build -trimpath -o bin/db       ./cmd/db

# ── Step 2: Cross-compile inside a Docker container ──────────────────────────
# Produces: bin/linux-amd64/{webhook,operator}  bin/linux-arm64/{webhook,operator}
# Uses a Go container so the build env is isolated from the local toolchain.
GO_IMAGE ?= golang:1.26-alpine

build-linux: web-build
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
	           go build -trimpath -o bin/linux-'"$$arch"'/operator ./cmd/operator && \
	           go build -trimpath -o bin/linux-'"$$arch"'/console ./cmd/console && \
	           go build -trimpath -o bin/linux-'"$$arch"'/gateway ./cmd/gateway && \
	           go build -trimpath -o bin/linux-'"$$arch"'/db ./cmd/db' \
	  || exit 1; \
	done

# ── Protobuf codegen ──────────────────────────────────────────────────────────
# Generated files are COMMITTED; day-to-day builds never need protoc.
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc on PATH.
proto:
	protoc \
	  --go_out=. --go_opt=module=github.com/Wynn-hub/kubesentry \
	  --go-grpc_out=. --go-grpc_opt=module=github.com/Wynn-hub/kubesentry \
	  api/proto/gateway/v1/gateway.proto

# ── OpenAPI codegen ───────────────────────────────────────────────────────────
# Generated files are COMMITTED; day-to-day builds never need ogen.
# Requires: ogen on PATH (go install github.com/ogen-go/ogen/cmd/ogen@latest).
OGEN = $(shell go env GOPATH)/bin/ogen

$(OGEN):
	go install github.com/ogen-go/ogen/cmd/ogen@latest

api-gen: $(OGEN)
	$(OGEN) --target internal/console/dto --package dto --clean \
	  api/console/console-api.yaml

api-lint:
	python3 scripts/check_openapi_contract.py
	cd web && npm run generate:api

# ── Test & Lint ───────────────────────────────────────────────────────────────
# Quick run for local development — no report files produced.
test:
	go test ./... -count=1

lint:
	go vet ./...

# ── E2E Tests ─────────────────────────────────────────────────────────────────
# Build images tagged e2e-test (local only, no push to registry).
# Depends on build-linux so e2e images are built from the same binaries
# that will be published — the test validates the exact release artifact.
build-image-e2e: build-linux
	docker build -f Dockerfile.webhook -t wynnhub/kubesentry-webhook:e2e-test .
	docker build -f Dockerfile.operator -t wynnhub/kubesentry-operator:e2e-test .
	docker build -f Dockerfile.console -t wynnhub/kubesentry-console:e2e-test .
	docker build -f Dockerfile.gateway -t wynnhub/kubesentry-gateway:e2e-test .
	docker build -f Dockerfile.db -t wynnhub/kubesentry-db:e2e-test .
	docker build -f Dockerfile.console-web -t wynnhub/kubesentry-console-web:e2e-test .

# E2E traffic must bypass any local HTTP(S) proxy (e.g. clash): kubectl and
# client-go honor https_proxy, and a proxied connection to the local API
# server once stalled a single kubectl call for ~13 minutes.
E2E_ENV = NO_PROXY=kubernetes.docker.internal,localhost,127.0.0.1 \
          no_proxy=kubernetes.docker.internal,localhost,127.0.0.1

# Run E2E tests against docker-desktop k8s (requires e2e-test images).
# Quick run for local development — no report files produced.
test-e2e:
	$(E2E_ENV) go test ./test/e2e/... -v -tags e2e -timeout 30m

# ── Regression Test Reports ───────────────────────────────────────────────────
# Run unit tests and emit JUnit XML + HTML report to $(REPORT_DIR).
# The HTML report is generated even when tests fail so failures are inspectable.
test-report: $(GOTESTSUM)
	@mkdir -p $(REPORT_DIR)
	$(GOTESTSUM) \
	  --junitfile $(REPORT_DIR)/unit-tests.xml \
	  --jsonfile  $(REPORT_DIR)/unit-tests.json \
	  --format    pkgname \
	  -- ./... -count=1; \
	  EXIT=$$?; \
	  python3 scripts/junit2html.py \
	    $(REPORT_DIR)/unit-tests.xml \
	    $(REPORT_DIR)/unit-tests.html \
	    "KubeSentry Unit Tests — $(VERSION)"; \
	  exit $$EXIT

# Run E2E tests and emit JUnit XML + HTML report to $(REPORT_DIR).
test-e2e-report: $(GOTESTSUM)
	@mkdir -p $(REPORT_DIR)
	$(E2E_ENV) $(GOTESTSUM) \
	  --junitfile $(REPORT_DIR)/e2e-tests.xml \
	  --jsonfile  $(REPORT_DIR)/e2e-tests.json \
	  --format    pkgname \
	  -- ./test/e2e/... -v -tags e2e -timeout 30m; \
	  EXIT=$$?; \
	  python3 scripts/junit2html.py \
	    $(REPORT_DIR)/e2e-tests.xml \
	    $(REPORT_DIR)/e2e-tests.html \
	    "KubeSentry E2E Tests — $(VERSION)"; \
	  exit $$EXIT

# Full release gate: unit tests + build + E2E, all with report generation.
test-all: test-report test-web build-image-e2e test-e2e-report
	@echo ""
	@echo "All tests passed. Reports saved to $(REPORT_DIR)/"
	@printf "  %-40s (HTML)\n" "$(REPORT_DIR)/unit-tests.html"
	@printf "  %-40s (HTML)\n" "$(REPORT_DIR)/e2e-tests.html"
	@printf "  %-40s (JUnit XML)\n" "$(REPORT_DIR)/unit-tests.xml"
	@printf "  %-40s (JUnit XML)\n" "$(REPORT_DIR)/e2e-tests.xml"

# ── Local Deploy ──────────────────────────────────────────────────────────────
# One-shot: build → image → helm upgrade --install to the current kubectl context.
# Targets docker-desktop / kind / minikube where kubelet shares the local image store.
# Uses tag "local" with pullPolicy=Never so kubelet never tries to pull from a registry.
LOCAL_TAG        ?= local
LOCAL_NAMESPACE  ?= kubesentry-system
LOCAL_WEBHOOK_IMAGE     = $(REGISTRY)/kubesentry-webhook:$(LOCAL_TAG)
LOCAL_OPERATOR_IMAGE    = $(REGISTRY)/kubesentry-operator:$(LOCAL_TAG)
LOCAL_CONSOLE_IMAGE     = $(REGISTRY)/kubesentry-console:$(LOCAL_TAG)
LOCAL_GATEWAY_IMAGE     = $(REGISTRY)/kubesentry-gateway:$(LOCAL_TAG)
LOCAL_DB_IMAGE          = $(REGISTRY)/kubesentry-db:$(LOCAL_TAG)
LOCAL_CONSOLE_WEB_IMAGE = $(REGISTRY)/kubesentry-console-web:$(LOCAL_TAG)

# Build single-arch images (host arch only) for local k8s — faster than buildx.
build-image-local: build-linux
	docker build -f Dockerfile.webhook  -t $(LOCAL_WEBHOOK_IMAGE)  .
	docker build -f Dockerfile.operator -t $(LOCAL_OPERATOR_IMAGE) .
	docker build -f Dockerfile.console  -t $(LOCAL_CONSOLE_IMAGE)  .
	docker build -f Dockerfile.gateway  -t $(LOCAL_GATEWAY_IMAGE)  .
	docker build -f Dockerfile.db       -t $(LOCAL_DB_IMAGE)       .
	docker build -f Dockerfile.console-web -t $(LOCAL_CONSOLE_WEB_IMAGE) .

# Build images + helm install/upgrade in one shot.
# Two chart installs into the same namespace: kubesentry-gateway (gateway +
# console, the "platform" component per design D1) and kubesentry (webhook +
# operator, the per-cluster enforcement engine). gateway installs FIRST now,
# out of necessity: the member chart's gateway.address/caBundle/clusterName/
# registrationToken are chart-required (standalone deployment removed), and
# the registration token only exists once the gateway has issued it — so
# scripts/deploy-local.sh installs gateway, self-registers this cluster
# ("local"), harvests the token, then installs the member chart once with
# every gateway field already set. Mirrors test/e2e's TestMain bootstrap order.
deploy-local: build-image-local
	@REGISTRY=$(REGISTRY) LOCAL_TAG=$(LOCAL_TAG) LOCAL_NAMESPACE=$(LOCAL_NAMESPACE) ./scripts/deploy-local.sh

undeploy-local:
	helm uninstall kubesentry --namespace $(LOCAL_NAMESPACE) || true
	helm uninstall kubesentry-gateway --namespace $(LOCAL_NAMESPACE) || true
	kubectl delete namespace $(LOCAL_NAMESPACE) --ignore-not-found
	@# The chart's post-delete hook removes these CRDs in current releases. Keep
	@# this idempotent fallback for interrupted hooks and installations created
	@# by older chart versions.
	@# clusters.kubesentry.io / policydistributions.kubesentry.io no longer
	@# exist — the gateway is fully database-backed now (zero CRDs); these
	@# four remain the member chart's own CRDs, unrelated to that migration.
	@for crd in policies.kubesentry.io policygroups.kubesentry.io \
	            policyversions.kubesentry.io policyexceptions.kubesentry.io; do \
	  kubectl delete crd $$crd --ignore-not-found; \
	done

# Fast console dev loop: builds bin/console, runs it against the current
# kubeconfig context, and runs the vite dev server with hot reload — no
# Docker image / Helm deploy involved. Ctrl+C stops both. Only Go backend
# changes need a re-run; frontend edits hot-reload automatically.
dev-local:
	@./scripts/dev-local.sh

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
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.console \
	  --tag $(CONSOLE_IMAGE) \
	  --load \
	  .
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.gateway \
	  --tag $(GATEWAY_IMAGE) \
	  --load \
	  .
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.db \
	  --tag $(DB_IMAGE) \
	  --load \
	  .
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.console-web \
	  --tag $(CONSOLE_WEB_IMAGE) \
	  --load \
	  .

# Build and push multi-platform manifest to registry.
# Always pushes both a versioned tag and :latest.
image-push: .builder build-linux
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.webhook \
	  --tag $(WEBHOOK_IMAGE) \
	  --tag $(REGISTRY)/kubesentry-webhook:latest \
	  --push \
	  .
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.operator \
	  --tag $(OPERATOR_IMAGE) \
	  --tag $(REGISTRY)/kubesentry-operator:latest \
	  --push \
	  .
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.console \
	  --tag $(CONSOLE_IMAGE) \
	  --tag $(REGISTRY)/kubesentry-console:latest \
	  --push \
	  .
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.gateway \
	  --tag $(GATEWAY_IMAGE) \
	  --tag $(REGISTRY)/kubesentry-gateway:latest \
	  --push \
	  .
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.db \
	  --tag $(DB_IMAGE) \
	  --tag $(REGISTRY)/kubesentry-db:latest \
	  --push \
	  .
	docker buildx build \
	  --builder $(BUILDER) \
	  --platform $(PLATFORMS) \
	  --file Dockerfile.console-web \
	  --tag $(CONSOLE_WEB_IMAGE) \
	  --tag $(REGISTRY)/kubesentry-console-web:latest \
	  --push \
	  .

# ── Step 4: Package Helm chart ────────────────────────────────────────────────
helm-package:
	@mkdir -p $(DIST_DIR)
	helm lint $(CHART_DIR)
	helm lint $(GATEWAY_CHART_DIR)
	@# Package from a staged copy with image tags pinned to the release
	@# version, so `helm install --version x.y.z` pulls the matching images.
	@# The working tree keeps tag: "latest" per branch policy.
	rm -rf $(DIST_DIR)/chart-src $(DIST_DIR)/gateway-chart-src
	cp -R $(CHART_DIR) $(DIST_DIR)/chart-src
	cp -R $(GATEWAY_CHART_DIR) $(DIST_DIR)/gateway-chart-src
	sed 's/tag: "latest"/tag: "$(TAG)"/' $(CHART_DIR)/values.yaml > $(DIST_DIR)/chart-src/values.yaml
	sed 's/tag: "latest"/tag: "$(TAG)"/' $(GATEWAY_CHART_DIR)/values.yaml > $(DIST_DIR)/gateway-chart-src/values.yaml
	helm package $(DIST_DIR)/chart-src \
	  --version $(CHART_VERSION) \
	  --app-version $(TAG) \
	  --destination $(DIST_DIR)
	helm package $(DIST_DIR)/gateway-chart-src \
	  --version $(CHART_VERSION) \
	  --app-version $(TAG) \
	  --destination $(DIST_DIR)
	@echo "Chart: $(DIST_DIR)/kubesentry-$(CHART_VERSION).tgz"
	@echo "Chart: $(DIST_DIR)/kubesentry-gateway-$(CHART_VERSION).tgz"

# Push the packaged chart to Docker Hub as an OCI artifact.
# Requires prior login: helm registry login registry-1.docker.io -u $(REGISTRY)
helm-push: helm-package
	helm push $(DIST_DIR)/kubesentry-$(CHART_VERSION).tgz \
	  oci://registry-1.docker.io/$(REGISTRY)
	helm push $(DIST_DIR)/kubesentry-gateway-$(CHART_VERSION).tgz \
	  oci://registry-1.docker.io/$(REGISTRY)
	@echo "Chart pushed: oci://registry-1.docker.io/$(REGISTRY)/kubesentry:$(CHART_VERSION)"
	@echo "Chart pushed: oci://registry-1.docker.io/$(REGISTRY)/kubesentry-gateway:$(CHART_VERSION)"

# ── Full release pipeline ─────────────────────────────────────────────────────
release: test-all image-push helm-push
	@echo ""
	@echo "Release complete:"
	@echo "  webhook:  $(WEBHOOK_IMAGE)"
	@echo "  operator: $(OPERATOR_IMAGE)"
	@echo "  console:  $(CONSOLE_IMAGE)"
	@echo "  gateway:  $(GATEWAY_IMAGE)"
	@echo "  db:       $(DB_IMAGE)"
	@echo "  chart:    oci://registry-1.docker.io/$(REGISTRY)/kubesentry:$(CHART_VERSION)"
	@echo "  chart:    oci://registry-1.docker.io/$(REGISTRY)/kubesentry-gateway:$(CHART_VERSION)"

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
	go clean -testcache

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
	@printf "  %-22s %s\n" "build"           "Compile for local arch → bin/webhook, bin/operator"
	@printf "  %-22s %s\n" "test"            "Run unit tests (no report)"
	@printf "  %-22s %s\n" "test-e2e"        "Run E2E tests (no report, requires e2e-test images)"
	@printf "  %-22s %s\n" "lint"            "Run go vet"
	@printf "  %-22s %s\n" "tools"           "Install test tooling (gotestsum)"
	@echo ""
	@echo "Local Deploy:"
	@printf "  %-22s %s\n" "deploy-local"    "Build images + helm install to current kubectl context"
	@printf "  %-22s %s\n" "undeploy-local"  "helm uninstall + delete namespace"
	@printf "  %-22s %s\n" "build-image-local" "Build local-tagged images (no deploy)"
	@echo ""
	@echo "Regression Reports:"
	@printf "  %-22s %s\n" "test-report"     "Unit tests + HTML/JUnit report → dist/test-reports/"
	@printf "  %-22s %s\n" "test-e2e-report" "E2E tests + HTML/JUnit report  → dist/test-reports/"
	@printf "  %-22s %s\n" "test-all"        "Full regression: unit + e2e with reports (used by release)"
	@echo ""
	@echo "Release:"
	@printf "  %-22s %s\n" "build-linux"  "Cross-compile in Docker → bin/linux-amd64/, bin/linux-arm64/"
	@printf "  %-22s %s\n" "image"        "Build multi-platform images and load into local daemon"
	@printf "  %-22s %s\n" "image-push"   "Build and push multi-platform images to registry"
	@printf "  %-22s %s\n" "helm-package" "Lint and package both Helm charts → dist/*.tgz"
	@printf "  %-22s %s\n" "helm-push"    "Push both Helm charts to oci://registry-1.docker.io/\$$REGISTRY"
	@printf "  %-22s %s\n" "release"      "Full pipeline: test-all → image-push → helm-push"
	@echo ""
	@echo "Maintenance:"
	@printf "  %-22s %s\n" "login"        "Login to Docker Hub (run once before release)"
	@printf "  %-22s %s\n" "clean"        "Remove bin/, dist/, buildx builder, test cache"
	@echo ""
	@echo "Examples:"
	@printf "  %s\n" "  make release VERSION=v0.1.0"
	@printf "  %s\n" "  make release VERSION=test"
	@printf "  %s\n" "  make image VERSION=v1 REGISTRY=myrepo"
	@printf "  %s\n" "  make helm-package VERSION=v0.2.0"
