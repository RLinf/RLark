##@ Lint & format

.PHONY: lint fmt
lint: ## Lint Go (golangci-lint)
	@echo "Running golangci-lint (sdks/embodied-runtime-go)..."
	@go mod download
	@golangci-lint run

fmt: ## Format Go (gofmt)
	@echo "Running gofmt (sdks/embodied-runtime-go)..."
	@go mod download
	@gofmt -l -w ./

##@ Go targets

.PHONY: vet test
vet: ## Run go vet
	go vet ./...

test: ## Run tests
	go test -v -count=1 ./...

##@ Help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\n"} \
		/^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
