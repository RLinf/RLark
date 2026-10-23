PROTO_TREE := execution

GO_SDK_DIR ?= ../../sdks/execution-go

PROTO_SPECS = \
	common/v1/types.proto \
	runtime/v1/runtime.proto \
	workload/v1/workload.proto

# Checked-in stub trees, relative to the repository root.
PROTO_GENERATED_PATHS = \
	sdks/execution-go/gen

include ../toolchain.mk

##@ Proto generation

.PHONY: proto

proto: proto-go ## Generate all protobuf stubs
