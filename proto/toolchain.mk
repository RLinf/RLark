# Shared protobuf generator toolchain for every proto tree in this repository.
#
# Include this file from proto/<tree>/Makefile after defining:
#   PROTO_TREE             directory name of the tree under proto/
#   GO_SDK_DIR             SDK directory that receives the generated Go stubs
#   PROTO_SPECS            .proto files to compile, relative to the tree
#   PROTO_GENERATED_PATHS  checked-in stub trees, relative to the repository root
#
# Every tree must use the same pins: the generated stubs are checked in and the
# generator versions are part of their file headers, so a tree generated with a
# different toolchain produces a repository-wide diff on the next regeneration.

TOOLCHAIN_MK := $(abspath $(lastword $(MAKEFILE_LIST)))
PROTO_ROOT := $(patsubst %/,%,$(dir $(TOOLCHAIN_MK)))
REPO_ROOT := $(patsubst %/,%,$(dir $(PROTO_ROOT)))

# Pinned generator toolchain. The Go and Python stubs under sdks/ are checked in
# and embed their generator versions in the file headers, so running a different
# toolchain rewrites every generated file even when no .proto changed. Keep these
# in sync with:
#   sdks/embodied-runtime-go/gen/device/v1/device.pb.go       (protoc, protoc-gen-go)
#   sdks/embodied-runtime-go/gen/device/v1/device_grpc.pb.go  (protoc-gen-go-grpc)
#   sdks/execution-go/gen/common/v1/types.pb.go               (protoc, protoc-gen-go)
#   sdks/embodied-runtime-python/requirements-dev.txt         (grpcio-tools)
# protoc reports its own version as 36.2 while the generated headers spell it
# v7.36.2; both refer to the same protobuf release.
PROTOC_VERSION ?= 36.2
PROTOC_GEN_GO_VERSION ?= v1.36.12
PROTOC_GEN_GO_GRPC_VERSION ?= v1.6.2
# buf is built from source with `go install`, so its `go` directive must not
# exceed the Go toolchain available in CI, which pins a Go release and runs with
# GOTOOLCHAIN=local. v1.72.0 requires go 1.25.10; v1.73.0 already requires
# go 1.26.7. Bumping this pin requires a CI image with a new enough Go.
BUF_VERSION ?= v1.72.0

# Pinned tools are installed under proto/bin (gitignored) with version-stamped
# file names so bumping a version above forces a reinstall instead of silently
# reusing whatever is already on disk. The directory is shared by all trees, so
# `make protoc-install` only has to run once.
LOCALBIN ?= $(PROTO_ROOT)/bin
PROTOC ?= $(firstword $(wildcard $(LOCALBIN)/protoc) protoc)
PROTOC_GEN_GO := $(LOCALBIN)/protoc-gen-go-$(PROTOC_GEN_GO_VERSION)
PROTOC_GEN_GO_GRPC := $(LOCALBIN)/protoc-gen-go-grpc-$(PROTOC_GEN_GO_GRPC_VERSION)
BUF := $(LOCALBIN)/buf-$(BUF_VERSION)

# protoc release asset naming differs from `uname` output.
PROTOC_OS := $(patsubst darwin,osx,$(shell uname -s | tr '[:upper:]' '[:lower:]'))
PROTOC_ARCH := $(subst arm64,aarch_64,$(subst aarch64,aarch_64,$(subst amd64,x86_64,$(shell uname -m))))
PROTOC_ZIP := protoc-$(PROTOC_VERSION)-$(PROTOC_OS)-$(PROTOC_ARCH).zip
PROTOC_URL := https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/$(PROTOC_ZIP)

# Git input that `buf breaking` compares the working tree against. CI overrides
# this with the merge-request base commit, e.g. PROTO_BREAKING_REF=ref=<sha>.
PROTO_BREAKING_REF ?= branch=main

.DEFAULT_GOAL := proto

##@ Proto generation

.PHONY: proto-go

proto-go: protoc-check $(PROTOC_GEN_GO) $(PROTOC_GEN_GO_GRPC) ## Generate Go protobuf and gRPC stubs
	@mkdir -p $(GO_SDK_DIR)/gen
	$(PROTOC) \
		--plugin=protoc-gen-go=$(PROTOC_GEN_GO) \
		--plugin=protoc-gen-go-grpc=$(PROTOC_GEN_GO_GRPC) \
		--proto_path=. \
		--go_out=$(GO_SDK_DIR)/gen \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(GO_SDK_DIR)/gen \
		--go-grpc_opt=paths=source_relative \
		$(PROTO_SPECS)

##@ Proto toolchain

.PHONY: protoc-check protoc-install

protoc-check: ## Fail unless the pinned protoc version is available
	@found=$$($(PROTOC) --version 2>/dev/null | awk '{print $$2}'); \
	if [ "$$found" != "$(PROTOC_VERSION)" ]; then \
		echo "ERROR: protoc $(PROTOC_VERSION) is required, found \"$${found:-none}\" ($(PROTOC))."; \
		echo "The checked-in stubs embed the protoc version, so generating with a"; \
		echo "different release rewrites every file under sdks/. Install the pinned"; \
		echo "release into proto/bin with:"; \
		echo "    make protoc-install"; \
		echo "or point PROTOC at an existing protoc $(PROTOC_VERSION) binary."; \
		exit 1; \
	fi

protoc-install: $(LOCALBIN) ## Download the pinned protoc release into proto/bin
	@echo "Downloading $(PROTOC_URL)"
	@tmpdir=$$(mktemp -d) && trap 'rm -rf "$$tmpdir"' EXIT && \
		curl -fsSL --retry 3 --retry-connrefused -o "$$tmpdir/$(PROTOC_ZIP)" "$(PROTOC_URL)" && \
		unzip -qo "$$tmpdir/$(PROTOC_ZIP)" -d "$$tmpdir/extracted" && \
		install -m 0755 "$$tmpdir/extracted/bin/protoc" "$(LOCALBIN)/protoc" && \
		echo "Installed $(LOCALBIN)/protoc ($$($(LOCALBIN)/protoc --version))"

$(LOCALBIN):
	mkdir -p $(LOCALBIN)

$(PROTOC_GEN_GO): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	mv $(LOCALBIN)/protoc-gen-go $@

$(PROTOC_GEN_GO_GRPC): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
	mv $(LOCALBIN)/protoc-gen-go-grpc $@

$(BUF): | $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)
	mv $(LOCALBIN)/buf $@

##@ Proto compatibility

.PHONY: lint-proto proto-breaking verify-proto-generated

verify-proto-generated: ## Fail if the checked-in gRPC stubs are out of date
	@$(MAKE) proto $(MAKEOVERRIDES)
	@if ! git -C $(REPO_ROOT) diff --exit-code --stat -- $(PROTO_GENERATED_PATHS); then \
		echo ""; \
		echo "ERROR: the checked-in gRPC stubs are out of date."; \
		echo "They do not match proto/$(PROTO_TREE)/*.proto as generated by the"; \
		echo "pinned toolchain. Run 'make proto' and commit the result."; \
		exit 1; \
	fi
	@if [ -n "$$(git -C $(REPO_ROOT) ls-files --others --exclude-standard -- $(PROTO_GENERATED_PATHS))" ]; then \
		echo ""; \
		echo "ERROR: 'make proto' produced untracked files:"; \
		git -C $(REPO_ROOT) ls-files --others --exclude-standard -- $(PROTO_GENERATED_PATHS); \
		echo "Commit them or remove the stale .proto files that produce them."; \
		exit 1; \
	fi

lint-proto: $(BUF) ## Lint .proto files (buf lint)
	$(BUF) lint

proto-breaking: $(BUF) ## Fail if .proto changes break the published gRPC contract
	@echo "Comparing proto/$(PROTO_TREE) against .git#$(PROTO_BREAKING_REF)"
	@$(BUF) breaking --against "$(REPO_ROOT)/.git#$(PROTO_BREAKING_REF),subdir=proto/$(PROTO_TREE)" || { \
		echo ""; \
		echo "ERROR: incompatible .proto change detected."; \
		echo "The gRPC contract and the checked-in SDKs under sdks/ are public."; \
		echo "Add new fields or RPCs instead of renaming, renumbering or removing"; \
		echo "existing ones. See proto/$(PROTO_TREE)/buf.yaml."; \
		exit 1; \
	}

##@ Help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\n"} \
		/^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
