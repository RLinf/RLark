.PHONY: lint fmt

lint: ## Lint Go (golangci-lint)
	@echo "Running golangci-lint (sdks/execution-go)..."
	@go mod download
	@golangci-lint run

fmt: ## Format Go (gofmt)
	@echo "Running gofmt (sdks/execution-go)..."
	@go mod download
	@gofmt -l -w ./gen
