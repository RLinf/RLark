# Execution Protocol Definitions

Versioned gRPC contracts for execution components that may run outside the
RLark Agent process in the future.

## Service boundaries

| Service | Purpose |
| --- | --- |
| `WorkloadAdapter` | External implementation of Workload Info, Compile, and Observe |
| `RuntimeProvider` | Runtime object CRUD and complete object/instance snapshots |

Environment Resolver/Binder, Coordinator, Compatibility Mapper, and Instance
Projection remain in-process contracts. Workload Adapters may be built in or
remote. Both receive the complete typed Workload configuration: the common Pod
template, opaque workload parameters, and versioned carrier envelope. They are
solely responsible for interpreting it and returning one complete declarative
Plan, including Environment requirements with exact mutation targets, workload
units, process declarations, and subsequent observations. Task-local material
such as SSH authorized public keys is not part of this protocol.

The current negotiated protocol is major version 2. It requires complete Plan
transport and is intentionally incompatible with the earlier object-only Plan
shape.

The protocol is intentionally separate from the current internal Go types.
Adding a proto message does not enable a remote implementation or make this a
public API. Production clients and servers require conformance tests, mTLS,
deadlines, request IDs, audit logging, version negotiation, and recovery tests.

## Generate Go stubs

Install compatible `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc`
binaries, then run from the repository root:

```bash
make proto-execution
```

Generated files are written to `sdks/execution-go/gen`. Generated files are
committed so normal builds do not require the protobuf toolchain.

Applications should import only generated packages from the SDK, for example:

```go
import runtimev1 "github.com/rlinf/rlark/sdks/execution-go/gen/runtime/v1"
```

Application adapters translate between generated wire messages and
`pkg/agent/execution` internal values. Internal packages must not make protobuf
messages their domain model.
