# Variables

REGISTRY        ?= rlinf
VERSION         ?= latest
IMAGE_TAG       ?= $(VERSION)
BUILDX_PLATFORMS ?= linux/amd64
BASE_IMAGE ?= nginx:1.29.1

##@ Build

.PHONY: install build lint fmt
install: ## Install npm dependencies
	npm install

build: install ## Build frontend static files
	npm run build

lint: install ## Lint TypeScript/React (eslint)
	npx eslint src/

fmt: ## Format code (prettier)
	npx prettier@3.9.6 --write 'src/' 'index.html' 'package.json' 'vite.config.ts' 'tsconfig*.json'

##@ Docker

.PHONY: docker-build docker-push
docker-build: build ## Build rlark-ui image and load into local docker
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--load \
		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
		-t $(REGISTRY)/rlark-ui:$(IMAGE_TAG) \
		-f Dockerfile \
		../..

docker-push: build ## Build rlark-ui image and push to registry
	docker buildx build \
		--platform $(BUILDX_PLATFORMS) \
		--push \
		--build-arg BASE_IMAGE=$(BASE_IMAGE) \
		-t $(REGISTRY)/rlark-ui:$(IMAGE_TAG) \
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
	@echo "  BASE_IMAGE           Nginx base image (default: nginx:1.29.1)"
