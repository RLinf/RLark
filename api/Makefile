# Pinned code generator. The CRD manifests and deepcopy funcs are checked in and
# record this version in their headers, so every environment must use the same
# pin or the next regeneration rewrites every generated file.
CONTROLLER_TOOLS_VERSION ?= v0.18.0

# The generator is installed into the gitignored, repository-local api/bin under
# a version-stamped file name. A shared GOPATH/bin is not usable here: CI shares
# it across jobs and projects, so a file named `controller-gen` may be stale or
# not even executable while make still treats the install target as satisfied
# and skips the install, which then fails mid-generation with exit code 127.
LOCALBIN ?= $(CURDIR)/bin
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)

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
.PHONY: controller-gen print-controller-gen

controller-gen: $(CONTROLLER_GEN) ## Install the pinned controller-gen into api/bin

print-controller-gen: ## Print the path of the pinned controller-gen
	@echo $(CONTROLLER_GEN)

generate-manifests: $(CONTROLLER_GEN) ## Generate CRD YAML manifests
	mkdir -p $(CRD_DIR)
	$(CONTROLLER_GEN) object paths=./rlark.io/...
	$(CONTROLLER_GEN) crd:allowDangerousTypes=true,generateEmbeddedObjectMeta=true paths=./rlark.io/... output:crd:artifacts:config=$(CRD_DIR)

generate-clients: | $(LOCALBIN) ## Generate typed clients, informers, listers
	GOBIN=$(LOCALBIN) ./hack/generate-clients.sh --with-watch

generate-crd: generate-manifests generate-clients ## Generate CRD manifests and clients

samples: ## Verify sample CRs exist
	@test -d $(SAMPLES_DIR)

clean-samples: ## Remove sample CRs
	rm -rf $(SAMPLES_DIR)

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

# `go install` writes an unversioned name, so rename it to the version-stamped
# path make depends on. The final `--version` call is the executability check:
# if the installed file cannot be executed the install fails here instead of
# much later, in the middle of generation.
$(CONTROLLER_GEN): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
	mv $(LOCALBIN)/controller-gen $@
	$@ --version

##@ Help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\n"} \
		/^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
