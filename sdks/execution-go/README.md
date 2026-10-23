# RLark Execution Go SDK

Generated protobuf and gRPC bindings for the versioned RLark execution
protocols. Generate them from the repository root with:

```bash
make proto-execution
```

Do not edit files under `gen/` manually. Application code should translate
between these wire types and its internal execution domain types.
