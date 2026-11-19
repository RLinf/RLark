# Developer Guide

This section is for contributors changing RLark itself. Read the [Project Design Principles](design-principles.md), [Project Layout](project-layout.md), and [Technical Architecture](../architecture.md), then set up [Local Development](local-development.md). Product deployment and cluster onboarding belong in the [Administrator Guide](../admin-guide/index.md).

Before changing an API type, a controller status write, or a Gateway endpoint, read [API Conventions](api-conventions.md): it defines the shared terminology, field ownership, Phase and Condition rules, stable identity, tenant metadata, and the REST error, pagination, and filtering contracts.

Developer documentation is API-, CRD-, CLI-, source-, and debugging-first and does not depend on Web UI screenshots. User-facing capabilities must keep UI and API semantics aligned.

See [Runtime, Workload, and Environment Boundary](runtime-provider.md) for the internal execution architecture and compatibility rules.
