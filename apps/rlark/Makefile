# Variables

REGISTRY        ?= rlinf
VERSION         ?= latest
IMAGE_TAG       ?= $(VERSION)
BUILDX_PLATFORMS ?= linux/amd64
GO_BASE_IMAGE   ?= golang:1.26-alpine
RUNTIME_BASE_IMAGE ?= ubuntu:22.04
GOOS    ?= linux
GOARCH  ?= amd64

# Build-time version injection
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "unknown")
LDFLAGS    := -s -w \
	-X github.com/rlinf/rlark/apps/rlark/pkg/version.Version=$(VERSION) \
	-X github.com/rlinf/rlark/apps/rlark/pkg/version.Commit=$(COMMIT) \
	-X github.com/rlinf/rlark/apps/rlark/pkg/version.BuildTime=$(BUILD_TIME)

SERVER_BIN            := server
CONTROLLER_MANAGER_BIN := controller-manager
GATEWAY_BIN           := gateway
AGENT_BIN             := agent
NETWORK_SIDECAR_BIN   := network-sidecar
RLARKADM_BIN          := rlarkadm
SSHD_BIN              := rlark-sshd

##@ Lint & format

.PHONY: lint fmt
lint: ## Lint Go (golangci-lint)
	@echo "Running golangci-lint (apps/rlark)..."
	@go mod download
	@golangci-lint run

fmt: ## Format Go (gofmt)
	@echo "Running gofmt (apps/rlark)..."
	@go mod download
	@gofmt -l -w ./cmd ./pkg

##@ Go targets

.PHONY: all build
all: build ## Build all binaries (default)
build: $(SERVER_BIN) $(CONTROLLER_MANAGER_BIN) $(GATEWAY_BIN) $(AGENT_BIN) $(NETWORK_SIDECAR_BIN) $(RLARKADM_BIN) $(SSHD_BIN) ## Build all binaries

.PHONY: $(SERVER_BIN)
$(SERVER_BIN): ## Build server binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@/...

.PHONY: $(CONTROLLER_MANAGER_BIN)
$(CONTROLLER_MANAGER_BIN): ## Build controller-manager binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@/...

.PHONY: $(GATEWAY_BIN)
$(GATEWAY_BIN): ## Build gateway binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@/...

.PHONY: $(AGENT_BIN)
$(AGENT_BIN): ## Build agent binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@/...

.PHONY: $(NETWORK_SIDECAR_BIN)
$(NETWORK_SIDECAR_BIN): ## Build network-sidecar binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@/...

.PHONY: $(RLARKADM_BIN)
$(RLARKADM_BIN): ## Build rlarkadm binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@

.PHONY: $(SSHD_BIN)
$(SSHD_BIN): ## Build rlark-sshd binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/sshd

.PHONY: vet test clean
vet: ## Run go vet
	go vet ./...

test: ## Run tests
	go test -v -count=1 ./...

clean: ## Clean Go cache and bin/
	go clean -cache
	rm -rf bin/

##@ Docker

.PHONY: docker-build docker-push
docker-build: ## Build rlark image and load into local docker
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--load \
		--build-arg GO_BASE_IMAGE=$(GO_BASE_IMAGE) \
		--build-arg RUNTIME_BASE_IMAGE=$(RUNTIME_BASE_IMAGE) \
		-t $(REGISTRY)/rlark:$(IMAGE_TAG) \
		-f Dockerfile \
		../..

docker-push: ## Build rlark image and push to registry
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--push \
		--build-arg GO_BASE_IMAGE=$(GO_BASE_IMAGE) \
		--build-arg RUNTIME_BASE_IMAGE=$(RUNTIME_BASE_IMAGE) \
		-t $(REGISTRY)/rlark:$(IMAGE_TAG) \
		-f Dockerfile \
		../..

##@ Help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\n"} \
		/^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo ""
	@echo "  \033[1mVariables:\033[0m"
	@echo "  REGISTRY             Container registry (default: rlinf)"
	@echo "  IMAGE_TAG            Image tag (default: latest; also: VERSION)"
	@echo "  BUILDX_PLATFORMS     Multi-arch targets (default: linux/amd64)"
	@echo "  GO_BASE_IMAGE        Go builder image (default: golang:1.26-alpine)"
	@echo "  RUNTIME_BASE_IMAGE   Runtime base image (default: ubuntu:22.04)"
