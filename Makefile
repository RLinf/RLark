# rlark — Makefile
#
# Quick start:
#   make                          # build everything (lint-go, go-vet, go-build)
#   make docker-build             # build rlark image and load into local docker

GREP = grep -P
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	GREP = grep -E
endif

##@ Code style

.PHONY: lint-go lint-web lint-proto fmt-go fmt-web

lint-go: ## Lint all Go projects (golangci-lint)
	$(MAKE) -C api lint $(MAKEOVERRIDES)
	$(MAKE) -C apps/rlark lint $(MAKEOVERRIDES)
	$(MAKE) -C apps/embodied-runtime lint $(MAKEOVERRIDES)
	$(MAKE) -C sdks/embodied-runtime-go lint $(MAKEOVERRIDES)
	$(MAKE) -C sdks/execution-go lint $(MAKEOVERRIDES)
	$(MAKE) -C test/e2e lint $(MAKEOVERRIDES)

lint-web: ## Lint web UI (eslint)
	$(MAKE) -C apps/rlark-ui lint $(MAKEOVERRIDES)

lint-proto: ## Lint protobuf definitions (buf lint)
	$(MAKE) -C proto/embodied-runtime lint-proto $(MAKEOVERRIDES)
	$(MAKE) -C proto/execution lint-proto $(MAKEOVERRIDES)

fmt-go: ## Format all Go projects (gofmt)
	$(MAKE) -C api fmt $(MAKEOVERRIDES)
	$(MAKE) -C apps/rlark fmt $(MAKEOVERRIDES)
	$(MAKE) -C apps/embodied-runtime fmt $(MAKEOVERRIDES)
	$(MAKE) -C sdks/embodied-runtime-go fmt $(MAKEOVERRIDES)
	$(MAKE) -C sdks/execution-go fmt $(MAKEOVERRIDES)
	$(MAKE) -C test/e2e fmt $(MAKEOVERRIDES)

fmt-web: ## Format web UI (prettier)
	$(MAKE) -C apps/rlark-ui fmt $(MAKEOVERRIDES)

##@ Go targets

.PHONY: build test-go test-web test-e2e test-e2e-rlark go-tidy

build: ## Build all binaries (apps/rlark)
	$(MAKE) -C apps/rlark build $(MAKEOVERRIDES)

test-go: ## Run Go unit tests with coverage
	@mkdir -p coverage/go
	@for module in api apps/rlark apps/embodied-runtime test/e2e; do \
		name=$$(echo $$module | tr '/' '-'); \
		coverage=$$(cd $$module && realpath --relative-to=. $(CURDIR)/coverage/go/$$name.out); \
		(cd $$module && go test ./... -v -count=1 -short -covermode=atomic -coverprofile=$$coverage) || exit $$?; \
	done

test-web: ## Run web UI unit tests with coverage
	npm ci --prefix apps/rlark-ui
	npm run test:coverage --prefix apps/rlark-ui

test-e2e: test-e2e-rlark ## Run all implemented end-to-end suites

test-e2e-rlark: ## Provision Kind, deploy RLark, run smoke tests, collect evidence, and clean up
	$(MAKE) -C test/e2e test-rlark $(MAKEOVERRIDES)

go-tidy: ## Run go mod tidy on all workspace modules
	@for dir in $$( $(GREP) '^\t\./' go.work | sed 's/^\t//' ); do \
		echo "Running go mod tidy in $$dir"; \
		(cd $$dir && go mod tidy -x); \
	done

##@ Code generation

.PHONY: generate generate-crd generate-crd-schema-docs generate-web-models verify-generated proto proto-embodied-runtime proto-execution protoc-install proto-breaking verify-proto-generated

generate: generate-crd-schema-docs generate-web-models ## Generate CRD manifests, clients, docs, and web models

generate-crd: ## Generate CRD manifests and clients
	$(MAKE) -C api generate-crd $(MAKEOVERRIDES)

generate-crd-schema-docs: generate-crd ## Generate CRD schema reference docs
	go run ./apps/rlark/cmd/crd-api-docgen api/config/crd/bases apps/rlark/docs/reference/crd.md

generate-web-models: generate-crd ## Generate TypeScript models from CRD manifests
	npm ci --prefix apps/rlark-ui
	npm run generate:crd-models --prefix apps/rlark-ui

GENERATED_PATHS = api/config/crd/bases api/kubeclients 'api/rlark.io/**/zz_generated.deepcopy.go' apps/rlark/docs/reference/crd.md apps/rlark-ui/src/generated/rlark

verify-generated: ## Fail if generated code/manifests/docs are out of date
	@$(MAKE) generate $(MAKEOVERRIDES)
	@if ! git diff --exit-code --stat -- $(GENERATED_PATHS); then \
		echo ""; \
		echo "ERROR: generated artifacts are out of date."; \
		echo "The CRD manifests, deepcopy funcs, typed Go/TypeScript clients, or CRD reference docs"; \
		echo "do not match the Go types in api/rlark.io/. Run 'make generate' and"; \
		echo "commit the result."; \
		exit 1; \
	fi
	@if [ -n "$$(git ls-files --others --exclude-standard -- $(GENERATED_PATHS))" ]; then \
		echo ""; \
		echo "ERROR: 'make generate' produced untracked files:"; \
		git ls-files --others --exclude-standard -- $(GENERATED_PATHS); \
		echo "Commit them or remove the stale sources that produce them."; \
		exit 1; \
	fi

proto: proto-embodied-runtime proto-execution ## Generate all protobuf stubs

proto-embodied-runtime: ## Generate embodied-runtime protobuf stubs
	$(MAKE) -C proto/embodied-runtime proto $(MAKEOVERRIDES)

proto-execution: ## Generate execution protobuf stubs
	$(MAKE) -C proto/execution proto $(MAKEOVERRIDES)

protoc-install: ## Download the pinned protoc release into proto/bin
	$(MAKE) -C proto/embodied-runtime protoc-install $(MAKEOVERRIDES)

verify-proto-generated: ## Fail if the checked-in gRPC stubs are out of date
	$(MAKE) -C proto/embodied-runtime verify-proto-generated $(MAKEOVERRIDES)
	$(MAKE) -C proto/execution verify-proto-generated $(MAKEOVERRIDES)

proto-breaking: ## Fail if protobuf changes break the published gRPC contract
	$(MAKE) -C proto/embodied-runtime proto-breaking $(MAKEOVERRIDES)
	$(MAKE) -C proto/execution proto-breaking $(MAKEOVERRIDES)

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
