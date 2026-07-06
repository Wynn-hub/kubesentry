export GOROOT := /opt/homebrew/opt/go/libexec

.PHONY: build build-linux image image-push helm-package helm-push release login clean \
        test lint test-report test-e2e test-e2e-report test-all build-image-e2e tools help \
        build-image-local deploy-local undeploy-local

# ── Variables ─────────────────────────────────────────────────────────────────
REGISTRY ?= wynnhub
PLATFORMS ?= linux/amd64,linux/arm64
BUILDER   ?= kubesentry-builder
CHART_DIR  = charts/kubesentry
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

WEBHOOK_IMAGE  = $(REGISTRY)/kubesentry-webhook:$(TAG)
OPERATOR_IMAGE = $(REGISTRY)/kubesentry-operator:$(TAG)

CMDS = webhook operator

# Test tooling — auto-installed to GOPATH/bin on first use.
GOTESTSUM = $(shell go env GOPATH)/bin/gotestsum

$(GOTESTSUM):
	go install gotest.tools/gotestsum@latest

# Install all test tooling in one shot.
tools: $(GOTESTSUM)

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

# Run E2E tests against docker-desktop k8s (requires e2e-test images).
# Quick run for local development — no report files produced.
test-e2e:
	go test ./test/e2e/... -v -tags e2e -timeout 15m

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
	$(GOTESTSUM) \
	  --junitfile $(REPORT_DIR)/e2e-tests.xml \
	  --jsonfile  $(REPORT_DIR)/e2e-tests.json \
	  --format    pkgname \
	  -- ./test/e2e/... -v -tags e2e -timeout 15m; \
	  EXIT=$$?; \
	  python3 scripts/junit2html.py \
	    $(REPORT_DIR)/e2e-tests.xml \
	    $(REPORT_DIR)/e2e-tests.html \
	    "KubeSentry E2E Tests — $(VERSION)"; \
	  exit $$EXIT

# Full release gate: unit tests + build + E2E, all with report generation.
test-all: test-report build-image-e2e test-e2e-report
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
LOCAL_WEBHOOK_IMAGE  = $(REGISTRY)/kubesentry-webhook:$(LOCAL_TAG)
LOCAL_OPERATOR_IMAGE = $(REGISTRY)/kubesentry-operator:$(LOCAL_TAG)

# Build single-arch images (host arch only) for local k8s — faster than buildx.
build-image-local: build-linux
	docker build -f Dockerfile.webhook  -t $(LOCAL_WEBHOOK_IMAGE)  .
	docker build -f Dockerfile.operator -t $(LOCAL_OPERATOR_IMAGE) .

# Build images + helm install/upgrade in one shot.
deploy-local: build-image-local
	@echo "→ Deploying to kube context: $$(kubectl config current-context)"
	@# Adopt any pre-existing CRDs into the Helm release. Earlier chart
	@# versions kept CRDs under crds/ (un-managed by Helm); the current
	@# chart manages them under templates/crds/. Without this adoption,
	@# helm install errors out on "invalid ownership metadata" for each
	@# CRD that was left behind by a prior install.
	@for crd in policies.kubesentry.io policygroups.kubesentry.io \
	            policyversions.kubesentry.io policyexceptions.kubesentry.io; do \
	  if kubectl get crd $$crd >/dev/null 2>&1; then \
	    kubectl label  crd $$crd app.kubernetes.io/managed-by=Helm --overwrite >/dev/null; \
	    kubectl annotate crd $$crd \
	      meta.helm.sh/release-name=kubesentry \
	      meta.helm.sh/release-namespace=$(LOCAL_NAMESPACE) --overwrite >/dev/null; \
	  fi; \
	done
	helm upgrade --install kubesentry $(CHART_DIR) \
	  --namespace $(LOCAL_NAMESPACE) --create-namespace \
	  --set webhook.image.repository=$(REGISTRY)/kubesentry-webhook \
	  --set webhook.image.tag=$(LOCAL_TAG) \
	  --set webhook.image.pullPolicy=Never \
	  --set operator.image.repository=$(REGISTRY)/kubesentry-operator \
	  --set operator.image.tag=$(LOCAL_TAG) \
	  --set operator.image.pullPolicy=Never \
	  --wait --timeout 5m
	@echo ""
	@echo "Deployed. Inspect with:"
	@echo "  kubectl -n $(LOCAL_NAMESPACE) get pods"

undeploy-local:
	helm uninstall kubesentry --namespace $(LOCAL_NAMESPACE) || true
	kubectl delete namespace $(LOCAL_NAMESPACE) --ignore-not-found

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
release: test-all image-push helm-push
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
	@printf "  %-22s %s\n" "helm-package" "Lint and package Helm chart → dist/kubesentry-<version>.tgz"
	@printf "  %-22s %s\n" "helm-push"    "Push Helm chart to oci://registry-1.docker.io/\$$REGISTRY"
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
