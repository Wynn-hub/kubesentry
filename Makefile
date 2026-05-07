export GOROOT := /opt/homebrew/Cellar/go/1.26.2/libexec

.PHONY: generate build test lint docker-build docker-push

REGISTRY ?= your-registry
TAG ?= latest

generate:
	go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.1 \
		object:headerFile="" \
		paths="./internal/api/..."

build:
	go build -o bin/webhook ./cmd/webhook
	go build -o bin/operator ./cmd/operator

test:
	go test ./... -v -count=1

lint:
	go vet ./...

docker-build:
	docker build -f Dockerfile.webhook -t $(REGISTRY)/kubesentry-webhook:$(TAG) .
	docker build -f Dockerfile.operator -t $(REGISTRY)/kubesentry-operator:$(TAG) .

docker-push:
	docker push $(REGISTRY)/kubesentry-webhook:$(TAG)
	docker push $(REGISTRY)/kubesentry-operator:$(TAG)
