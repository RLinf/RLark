CONTROLLER_GEN ?= $(shell go env GOPATH)/bin/controller-gen
CRD_DIR ?= config/crd/bases
SAMPLES_DIR ?= config/samples

##@ Lint & format

.PHONY: lint fmt
lint: ## Lint Go (golangci-lint)
	@echo "Running golangci-lint (api)..."
	@go mod download
	@golangci-lint run

fmt: ## Format Go (gofmt)
	@echo "Running gofmt (api)..."
	@go mod download
	@gofmt -l -w ./

##@ Go targets

.PHONY: vet test
vet: ## Run go vet
	go vet ./...

test: ## Run tests
	go test -v -count=1 ./...

##@ Code generation

.PHONY: generate-manifests generate-clients generate-crd samples clean-samples

generate-manifests: $(CONTROLLER_GEN) ## Generate CRD YAML manifests
	mkdir -p $(CRD_DIR)
	$(CONTROLLER_GEN) object paths=./rlark.io/...
	$(CONTROLLER_GEN) crd:allowDangerousTypes=true paths=./rlark.io/... output:crd:artifacts:config=$(CRD_DIR)

generate-clients: ## Generate typed clients, informers, listers
	./hack/generate-clients.sh --with-watch

generate-crd: generate-manifests generate-clients ## Generate CRD manifests and clients

samples: ## Verify sample CRs exist
	@test -d $(SAMPLES_DIR)

clean-samples: ## Remove sample CRs
	rm -rf $(SAMPLES_DIR)

$(CONTROLLER_GEN):
	GOBIN=$(shell go env GOPATH)/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5

##@ Help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\n"} \
		/^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
