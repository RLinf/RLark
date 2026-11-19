# End-to-end testing

RLark's repository-level E2E framework is under `test/e2e`. Each scenario
declares its infrastructure requirements in code, and the RLark entry point
creates and destroys isolated Kind clusters for the run.

The shared framework supports arbitrary declared cluster roles and does not
define product APIs, images, or topology. Product adapters are kept under their
suite. The RLark entry point uses Kind: it creates uniquely named clusters for
the requirements declared by each scenario, exports per-run kubeconfigs,
collects logs, and deletes only clusters owned by that run. It does not connect
to pre-existing clusters.

Cluster roles and node counts are test requirements in source code, not global
environment settings. A provider must satisfy the scenario declaration and
must not change what is tested based on external configuration.

The shared scenario runner owns provisioning, preparation, execution, evidence
collection, product cleanup, and infrastructure destruction. Collection and
cleanup receive a separate bounded context so a test timeout does not suppress
failure evidence or cleanup. `E2E_RETAIN_ENVIRONMENT` is the only environment-retention
switch.

The framework's contributor README is `test/e2e/README.md`. It describes the
directory structure, configurable image values, commands, resource ownership
rules, and artifact contract.

Quick commands:

```bash
# Create Kind, deploy RLark, verify, collect evidence, and clean up
make test-e2e-rlark
```

This is the only RLark E2E entry point. It builds `rlarkadm` and the default
RLark/UI images, creates an isolated Kind cluster, loads the images,
installs RLark using the Kubernetes management API, verifies API discovery and
component availability, collects evidence, uninstalls RLark, and removes the
cluster. Set `RLARK_SMOKE_IMAGE` and `RLARK_SMOKE_UI_IMAGE` to choose other
build tags. To use existing immutable local images, also set
`RLARK_SMOKE_BUILD_IMAGES=false`; the entry point validates and loads those
exact images. Restricted environments can set `RLARK_SMOKE_GO_BASE_IMAGE`,
`RLARK_SMOKE_RUNTIME_BASE_IMAGE`, and `RLARK_SMOKE_UI_BASE_IMAGE`; repository
defaults do not embed a private registry.

Product deployment inputs such as images, namespaces, feature settings, and
upgrade versions belong to the scenario that uses them, not global framework
configuration. Scenarios must use immutable image identities, unique run
labels, bounded context-aware polling, and ownership-proven cleanup. The test
framework never authorizes deleting unlabelled resources or product finalizers.
