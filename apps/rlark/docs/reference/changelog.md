# Release Notes / Changelog

RLark does not currently publish a stable release. The repository's `main` branch is a development snapshot; release notes will be published with repository releases when they become available. Do not infer released `0.1.x` versions from development metadata.

## Development Compatibility

| Source | Control plane / Agent | Kubernetes data plane | kcp | PostgreSQL | Status |
|--------|-----------------------|-----------------------|-----|------------|--------|
| `main` | Build all RLark components from the same commit | 1.31 (kind development environment) | 0.30 | 15 | Development only; no upgrade guarantee |

Other combinations are unverified. Keep the control plane and all Agents on the same commit; mixed RLark revisions are unsupported.

- [GitHub Releases](https://github.com/RLinf/RLark/releases)
- [Repository commit history](https://github.com/RLinf/RLark/commits/main/)
