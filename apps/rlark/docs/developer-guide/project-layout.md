# Project Layout

- `api/`: CRD types and generated Kubernetes clients.
- `apps/rlark/`: control plane, agents, CLI tools, and this documentation.
- `apps/rlark-ui/`: React and TypeScript web console. CRD TypeScript models under
  `src/generated/rlark/` are generated from `api/config/crd/bases` and committed.
- `apps/embodied-runtime/`: device plugins and robot or camera controllers.
- `proto/` and `sdks/`: runtime protocols and client SDKs.

Read the [Project Design Principles](design-principles.md) before changing the architecture or behavior. See the repository `AGENTS.md` and `CONTRIBUTING.md` for current agent, build, and contribution rules.
