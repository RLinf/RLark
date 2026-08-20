# rlark — Makefile
#
# Quick start:
#   make                          # build everything (lint-go, go-vet, go-build)
#   make docker-build             # build rlark image and load into local docker

##@ Code style

.PHONY: lint-go lint-web fmt-go fmt-web

lint-go: ## Lint all Go projects (golangci-lint)
	$(MAKE) -C api lint $(MAKEOVERRIDES)
	$(MAKE) -C apps/rlark lint $(MAKEOVERRIDES)
	$(MAKE) -C apps/embodied-runtime lint $(MAKEOVERRIDES)
	$(MAKE) -C sdks/embodied-runtime-go lint $(MAKEOVERRIDES)

lint-web: ## Lint web UI (eslint)
	$(MAKE) -C apps/rlark-ui lint $(MAKEOVERRIDES)

fmt-go: ## Format all Go projects (gofmt)
	$(MAKE) -C api fmt $(MAKEOVERRIDES)
	$(MAKE) -C apps/rlark fmt $(MAKEOVERRIDES)
	$(MAKE) -C apps/embodied-runtime fmt $(MAKEOVERRIDES)
	$(MAKE) -C sdks/embodied-runtime-go fmt $(MAKEOVERRIDES)

fmt-web: ## Format web UI (prettier)
	$(MAKE) -C apps/rlark-ui fmt $(MAKEOVERRIDES)

##@ Go targets

.PHONY: build go-tidy

build: ## Build all binaries (apps/rlark)
	$(MAKE) -C apps/rlark build $(MAKEOVERRIDES)

go-tidy: ## Run go mod tidy on all workspace modules
	@for dir in $$(grep '^\t\./' go.work | sed 's/^\t//'); do \
		(cd $$dir && go mod tidy); \
	done

##@ Code generation

.PHONY: generate generate-crd generate-crd-schema-docs proto

generate: generate-crd generate-crd-schema-docs ## Generate CRD manifests, clients, and schema docs

generate-crd: ## Generate CRD manifests and clients
	$(MAKE) -C api generate-crd $(MAKEOVERRIDES)

generate-crd-schema-docs: generate-crd ## Generate CRD schema reference docs
	go run ./apps/rlark/cmd/crd-api-docgen api/config/crd/bases apps/rlark/docs/reference/crd.md

proto: ## Generate protobuf stubs
	$(MAKE) -C proto/embodied-runtime proto $(MAKEOVERRIDES)

##@ Docker

.PHONY: docker-build docker-push-rlark docker-build-ui docker-push-rlark-ui docker-build-embodied-runtime docker-push-embodied-runtime

docker-build-rlark: ## Build rlark image and load into local docker
	$(MAKE) -C apps/rlark docker-build $(MAKEOVERRIDES)

docker-push-rlark: ## Build rlark image and push to registry
	$(MAKE) -C apps/rlark docker-push $(MAKEOVERRIDES)

docker-build-rlark-ui: ## Build rlark-ui image and load into local docker
	$(MAKE) -C apps/rlark-ui docker-build $(MAKEOVERRIDES)

docker-push-rlark-ui: ## Build rlark-ui image and push to registry
	$(MAKE) -C apps/rlark-ui docker-push $(MAKEOVERRIDES)

docker-build-embodied-runtime: ## Build embodied-runtime image and load into local docker
	$(MAKE) -C apps/embodied-runtime docker-build $(MAKEOVERRIDES)

docker-push-embodied-runtime: ## Build embodied-runtime image and push to registry
	$(MAKE) -C apps/embodied-runtime docker-push $(MAKEOVERRIDES)

##@ Help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\n"} \
		/^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
