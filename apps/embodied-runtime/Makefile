# Variables

REGISTRY        ?= rlinf
VERSION         ?= latest
IMAGE_TAG       ?= $(VERSION)
BUILDX_PLATFORMS ?= linux/amd64
GO_BASE_IMAGE   ?= golang:1.26-alpine
RUNTIME_BASE_IMAGE ?= alpine:3.21
GOOS    ?= linux
GOARCH  ?= amd64
APK_MIRROR ?=
APT_MIRROR ?=
LDFLAGS := -s -w

DEVPLUGIN_BIN   := device-plugin
ROS_CTRL_BIN    := ros-controller
ROS2_CTRL_BIN   := ros2-controller
ROSCTR_BIN      := rosctr
CAM_CTRL_BIN    := camera-controller
CAMCTR_BIN      := camctr
DEVINIT_BIN     := devinit

RUNTIME_OUTPUT_IMAGE       := $(REGISTRY)/embodied-runtime
CAMERA_BASE_OUTPUT_IMAGE   := $(REGISTRY)/camera-base
ROS_BASE_OUTPUT_IMAGE      := $(REGISTRY)/ros-base
ROS2_BASE_OUTPUT_IMAGE     := $(REGISTRY)/ros2-base

##@ Lint & format

.PHONY: lint fmt
lint: ## Lint Go (golangci-lint)
	@echo "Running golangci-lint (apps/embodied-runtime)..."
	@go mod download
	@golangci-lint run

fmt: ## Format Go (gofmt)
	@echo "Running gofmt (apps/embodied-runtime)..."
	@go mod download
	@gofmt -l -w ./

##@ Go targets

.PHONY: all build
all: build vet ## Build and vet (default)
build: $(DEVPLUGIN_BIN) $(ROS_CTRL_BIN) $(ROS2_CTRL_BIN) $(ROSCTR_BIN) $(CAM_CTRL_BIN) $(CAMCTR_BIN) $(DEVINIT_BIN) ## Build all binaries

.PHONY: $(DEVPLUGIN_BIN)
$(DEVPLUGIN_BIN): ## Build device-plugin binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@

.PHONY: $(ROS_CTRL_BIN)
$(ROS_CTRL_BIN): ## Build ros-controller binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@

.PHONY: $(ROS2_CTRL_BIN)
$(ROS2_CTRL_BIN): ## Build ros2-controller binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@

.PHONY: $(ROSCTR_BIN)
$(ROSCTR_BIN): ## Build rosctr binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@

.PHONY: $(CAM_CTRL_BIN)
$(CAM_CTRL_BIN): ## Build camera-controller binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@

.PHONY: $(CAMCTR_BIN)
$(CAMCTR_BIN): ## Build camctr binary
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@

.PHONY: $(DEVINIT_BIN)
$(DEVINIT_BIN): ## Build devinit binary (init-container device setup tool)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(LDFLAGS)" -o bin/$@ ./cmd/$@

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
docker-build: ## Build embodied-runtime image and load into local docker
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--load \
		--build-arg GO_BASE_IMAGE=$(GO_BASE_IMAGE) \
		--build-arg RUNTIME_BASE_IMAGE=$(RUNTIME_BASE_IMAGE) \
		--build-arg APK_MIRROR=$(APK_MIRROR) \
		-t $(RUNTIME_OUTPUT_IMAGE):$(IMAGE_TAG) \
		-f Dockerfile \
		../..

docker-push: ## Build embodied-runtime image and push to registry
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--push \
		--build-arg GO_BASE_IMAGE=$(GO_BASE_IMAGE) \
		--build-arg RUNTIME_BASE_IMAGE=$(RUNTIME_BASE_IMAGE) \
		--build-arg APK_MIRROR=$(APK_MIRROR) \
		-t $(RUNTIME_OUTPUT_IMAGE):$(IMAGE_TAG) \
		-f Dockerfile \
		../..

.PHONY: docker-camera-build docker-camera-push
docker-camera-build: ## Build camera-runtime base image (load into local docker)
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--load \
		--build-arg RUNTIME_BASE_IMAGE=$(RUNTIME_BASE_IMAGE) \
		--build-arg APK_MIRROR=$(APK_MIRROR) \
		-t $(CAMERA_BASE_OUTPUT_IMAGE):$(IMAGE_TAG) \
		-f runtimes/camera-base.dockerfile \
		.

docker-camera-push: ## Build camera-runtime base image and push to registry
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--push \
		--build-arg RUNTIME_BASE_IMAGE=$(RUNTIME_BASE_IMAGE) \
		--build-arg APK_MIRROR=$(APK_MIRROR) \
		-t $(CAMERA_BASE_OUTPUT_IMAGE):$(IMAGE_TAG) \
		-f runtimes/camera-base.dockerfile \
		.

.PHONY: docker-ros-build docker-ros-push
docker-ros-build: ## Build ros-runtime base image (load into local docker)
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--load \
		--build-arg RUNTIME_BASE_IMAGE=$(RUNTIME_BASE_IMAGE) \
		--build-arg APT_MIRROR=$(APT_MIRROR) \
		-t $(ROS_BASE_OUTPUT_IMAGE):$(IMAGE_TAG) \
		-f runtimes/ros-base.dockerfile \
		.

docker-ros-push: ## Build ros-runtime base image and push to registry
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--push \
		--build-arg RUNTIME_BASE_IMAGE=$(RUNTIME_BASE_IMAGE) \
		--build-arg APT_MIRROR=$(APT_MIRROR) \
		-t $(ROS_BASE_OUTPUT_IMAGE):$(IMAGE_TAG) \
		-f runtimes/ros-base.dockerfile \
		.

.PHONY: docker-ros2-build docker-ros2-push
docker-ros2-build: ## Build ros2-runtime base image (load into local docker)
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--load \
		--build-arg RUNTIME_BASE_IMAGE=$(RUNTIME_BASE_IMAGE) \
		--build-arg APT_MIRROR=$(APT_MIRROR) \
		-t $(ROS2_BASE_OUTPUT_IMAGE):$(IMAGE_TAG) \
		-f runtimes/ros2-base.dockerfile \
		.

docker-ros2-push: ## Build ros2-runtime base image and push to registry
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--push \
		--build-arg RUNTIME_BASE_IMAGE=$(RUNTIME_BASE_IMAGE) \
		--build-arg APT_MIRROR=$(APT_MIRROR) \
		-t $(ROS2_BASE_OUTPUT_IMAGE):$(IMAGE_TAG) \
		-f runtimes/ros2-base.dockerfile \
		.

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
	@echo "  RUNTIME_BASE_IMAGE   Runtime base image (default: alpine:3.21)"
	@echo "  APK_MIRROR           Alpine apk mirror (embodied-runtime + camera-base)"
	@echo "  APT_MIRROR           Debian apt mirror (ros-base + ros2-base)"
