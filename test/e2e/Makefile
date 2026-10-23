GO ?= go
ARTIFACTS ?= $(abspath $(CURDIR)/../../_artifacts/e2e)
RLARK_SMOKE_IMAGE ?= rlark-e2e/rlark:local
RLARK_SMOKE_UI_IMAGE ?= rlark-e2e/rlark-ui:local
RLARK_SMOKE_BUILD_IMAGES ?= true
RLARK_SMOKE_GO_BASE_IMAGE ?= golang:1.26-alpine
RLARK_SMOKE_RUNTIME_BASE_IMAGE ?= ubuntu:22.04
RLARK_SMOKE_UI_BASE_IMAGE ?= nginx:1.29.1

.PHONY: lint fmt test-rlark

lint: ## Lint E2E Go code (golangci-lint)
	@echo "Running golangci-lint (test/e2e)..."
	@$(GO) mod download
	@golangci-lint run

fmt: ## Format E2E Go code (gofmt)
	@echo "Running gofmt (test/e2e)..."
	@$(GO) mod download
	@gofmt -l -w ./framework ./rlark ./embodied-runtime ./integration

test-rlark:
	$(MAKE) -C ../../apps/rlark rlarkadm
	@if [ "$(RLARK_SMOKE_BUILD_IMAGES)" = "true" ]; then \
		docker buildx build --platform linux/amd64 --load \
			--build-arg GO_BASE_IMAGE="$(RLARK_SMOKE_GO_BASE_IMAGE)" \
			--build-arg RUNTIME_BASE_IMAGE="$(RLARK_SMOKE_RUNTIME_BASE_IMAGE)" \
			-t "$(RLARK_SMOKE_IMAGE)" -f ../../apps/rlark/Dockerfile ../..; \
	fi
	@if [ "$(RLARK_SMOKE_BUILD_IMAGES)" = "true" ]; then \
		$(MAKE) -C ../../apps/rlark-ui build; \
		docker buildx build --platform linux/amd64 --load \
			--build-arg BASE_IMAGE="$(RLARK_SMOKE_UI_BASE_IMAGE)" \
			-t "$(RLARK_SMOKE_UI_IMAGE)" -f ../../apps/rlark-ui/Dockerfile ../..; \
	fi
	E2E_RUN=1 E2E_ARTIFACTS=$(ARTIFACTS) \
		RLARK_SMOKE_IMAGE=$(RLARK_SMOKE_IMAGE) RLARK_SMOKE_UI_IMAGE=$(RLARK_SMOKE_UI_IMAGE) \
		$(GO) test ./rlark/smoke/... -count=1 -v -timeout=45m
