# End-to-end test framework

This module provides the repository-level E2E framework. Suites are separated
by product boundary:

- `framework`: product-neutral run configuration, arbitrary named Kubernetes
  clients, polling, run identity, artifact paths, and ownership checks;
- `rlark/framework`: RLark API client initialization and Kind provider configuration;
- `rlark`: RLark scenarios and product-specific fixtures;
- `embodied-runtime`: embodied-runtime scenarios;
- `integration`: scenarios crossing product boundaries.

The shared framework does not define a topology or know about any product API.
The default provider creates isolated Kind clusters for requirements declared
inside each scenario. There is no global default topology or configurable
cluster count. For example, a smoke scenario may request one management cluster
and one workload cluster, while an upgrade scenario may request additional
clusters. Providers receive this declaration as data and do not embed role
names or infer requirements from environment variables.

## Configuration

Shared runner variables use the `E2E_` prefix. RLark framework variables use
`RLARK_E2E_`, while each scenario owns its product inputs. The RLark entry point
always provisions new Kind clusters; it does not run against pre-existing
clusters.

| Variable | Default | Purpose |
| --- | --- | --- |
| `E2E_REPO_ROOT` | discovered from `go.work` | repository root |
| `E2E_ARTIFACTS` | `<repo>/_artifacts/e2e` | artifact output root |
| `E2E_TIMEOUT` | `5m` | default scenario timeout |
| `E2E_CLEANUP_TIMEOUT` | `2m` | cleanup convergence timeout |
| `E2E_RETAIN_ENVIRONMENT` | false | retain product resources and infrastructure for debugging |
| `RLARK_E2E_KIND_BINARY` | `kind` | Kind executable |
| `RLARK_E2E_KIND_NODE_IMAGE` | empty | optional immutable Kind node image |

Product deployment inputs such as images, namespaces, feature settings, and
upgrade versions are declared and validated by the scenario that needs them.
They are not global framework configuration. `RLARK_SMOKE_IMAGE`,
`RLARK_SMOKE_UI_IMAGE`, `RLARK_SMOKE_RLARKADM`, and `RLARK_SMOKE_TIMEOUT`
belong only to the smoke scenario.

## Running

Run the complete RLark smoke lifecycle:

```bash
make test-e2e-rlark
```

The command builds `rlarkadm` and the default RLark/UI images, creates
an isolated Kind cluster, loads the two local scenario images, deploys RLark
with the Kubernetes management API, verifies API discovery and component
availability, captures evidence, uninstalls RLark, and deletes the cluster.
`RLARK_SMOKE_TIMEOUT` defaults to `20m`.

The default images are `rlark-e2e/rlark:local` and
`rlark-e2e/rlark-ui:local`. Set `RLARK_SMOKE_IMAGE` and
`RLARK_SMOKE_UI_IMAGE` to choose other build tags. To use existing immutable
local images without rebuilding, also set `RLARK_SMOKE_BUILD_IMAGES=false`;
the entry point validates and loads those exact images without substitution.
When building, `RLARK_SMOKE_GO_BASE_IMAGE`,
`RLARK_SMOKE_RUNTIME_BASE_IMAGE`, and `RLARK_SMOKE_UI_BASE_IMAGE` select the
three base images. This supports restricted registries without embedding a
registry address in the repository.

The scenario runner owns the lifecycle: `Provision`, product `Prepare`,
`Execute`, product `Collect`, infrastructure `Collect`, product `Cleanup`, then
infrastructure `Destroy`. Collection and cleanup use a fresh bounded context
even when the test context has expired. `E2E_RETAIN_ENVIRONMENT` is the single switch
for retaining provisioned infrastructure. Kind cluster names include the run
ID. Partial provisioning failures roll back clusters already created by that
run.

## Writing scenarios

Each scenario creates a shared `framework.Run`. Product suites compose it with
their own environment adapter; they must not add product contracts to the
shared package. A run provides:

- a unique run ID;
- DNS-safe resource name prefixes;
- suite-defined ownership labels (`e2e.rlark.io/run-id` and
  `e2e.rlark.io/spec` for RLark);
- a per-scenario artifact directory;
- an exact label selector for listing owned resources.

Cleanup of scenario-created business resources must list by the run ID and then
prove ownership before deletion. Names alone are not ownership proof. A scenario
that owns an entire ephemeral cluster may use its deployment manifest and
dedicated namespace as ownership proof. Do not remove product finalizers.
Persistent PVC deletion additionally requires the expected label and UID and
proof that no Pod references the claim.

Use `framework.Poll` to re-read current state. Do not infer correctness from a
single watch event or use long fixed sleeps. Collect object state, events, and
logs before cleanup when a scenario fails; never persist Secret data.

Tests that mutate shared components (CRDs, Controller Manager, Cluster Agents,
or cluster-wide configuration) must run serially. Deterministic timing and
channel-pressure behavior belongs in unit tests, not unstable E2E windows.
