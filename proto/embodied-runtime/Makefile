GO_SDK_DIR ?= ../../sdks/embodied-runtime-go
PY_SDK_DIR ?= ../../sdks/embodied-runtime-python

PROTO_SPECS = \
	roscontroller/v1/robot.proto \
	cameracontroller/v1/camera.proto \
	device/v1/device.proto

##@ Proto generation

.PHONY: proto proto-go proto-python
proto: proto-go proto-python ## Generate all protobuf stubs

proto-go: ## Generate Go protobuf stubs
	protoc \
		--proto_path=. \
		--go_out=$(GO_SDK_DIR)/gen \
		--go_opt=paths=source_relative \
		--go-grpc_out=$(GO_SDK_DIR)/gen \
		--go-grpc_opt=paths=source_relative \
		$(PROTO_SPECS)

proto-python: ## Generate Python protobuf stubs
	cd $(PY_SDK_DIR) && python3 scripts/gen_proto.py

##@ Help

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\n"} \
		/^[a-zA-Z_-]+:.*##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
