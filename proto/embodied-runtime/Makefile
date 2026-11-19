PROTO_TREE := embodied-runtime

GO_SDK_DIR ?= ../../sdks/embodied-runtime-go
PY_SDK_DIR ?= ../../sdks/embodied-runtime-python

# Interpreter used to run the Python generator. Override it when the default
# python3 is not the interpreter that has the pinned grpcio-tools installed,
# e.g. PYTHON=/path/to/venv/bin/python.
PYTHON ?= python3

PROTO_SPECS = \
	roscontroller/v1/robot.proto \
	cameracontroller/v1/camera.proto \
	device/v1/device.proto

# Checked-in stub trees, relative to the repository root.
PROTO_GENERATED_PATHS = \
	sdks/embodied-runtime-go/gen \
	sdks/embodied-runtime-python/embodied_runtime/gen

include ../toolchain.mk

##@ Proto generation

.PHONY: proto proto-python

proto: proto-go proto-python ## Generate all protobuf stubs

proto-python: ## Generate Python protobuf stubs
	cd $(PY_SDK_DIR) && $(PYTHON) scripts/gen_proto.py
