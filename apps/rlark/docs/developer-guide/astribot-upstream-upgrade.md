# Astribot review of upstream RLark

The reviewed range is `9ad4a21..d06745e`. The feature changes came from `8c3a72d`, and `d06745e` corrected the release workflow and added the v0.1.0 changelog. The Astribot branch contains `d06745e` and all 211 paths added in the upstream range. Keep the integration evidence below separate from acceptance on a deployed environment.

| Area | Branch integration | Environment acceptance |
| --- | --- | --- |
| Data-plane batch upgrades | The shared upgrade page, Gateway API, both CRD schemas, Controller Manager reconciler and Agent executor are present. Astribot management identities now grant the Gateway access to upgrade jobs and the Controller Manager access to reconcile and report them. The data-plane Agent's cluster-scoped management role already includes the child resource and its status. | Apply the CRDs and RBAC before enabling use. Use an approved `rlark-upgrade` image and test a disposable Kubernetes cluster, including concurrency, timeout, failure threshold and rollback. Verify Agent arguments, volume mounts, credentials and topology before expanding to Astribot-managed clusters. |
| Node lifecycle | Node management retains heartbeat and schedulability, cordon, drain progress, undrain, recovery and removal. Both confirmation dialogs now describe explicit Job start after a drain. Astribot station removal remains a separate flow. | On an idle test node, verify drain completion, then resume scheduling and confirm affected tasks stay stopped until their Job is explicitly started. Verify the station removal flow separately. |
| Scheduling and diagnosis | Job creation retains cluster and node selection, capacity and schedulability filtering, and `/dev/shm` configuration. Worker diagnostics retain scheduling, storage and image-pull reasons. The Kubernetes owner-chain validation and status projection also cover Astribot DaemonSets and per-node task instances. | Run Kubernetes jobs that exercise node selection, a waiting worker and the shared-memory mount. Compare the Pod and Task status with the worker UI. Verify a DaemonSet owner UID mismatch and replacement Pod do not leak old status, and that per-node instances update and clear. |
| Agent, recovery and deployment | Startup configuration validation, dependency and permission checks, periodic repair, ownership checks, Helm charts and the release workflow are present. The control-plane chart copies match the canonical CRD schemas. | Exercise Agent restart and self-repair, disk-pressure recovery and ordinary controller startup in a test cluster. Render and install both charts in an isolated environment to check the upstream deployment path. Astribot retains its own deployment process. |

The standard Astribot release applies CRDs and the generated management roles in its `crds-rbac` stage. The Astribot image inventory and the upstream v0.1.0 published image list do not include `rlark-upgrade`. Build and publish an approved image using the repository's `docker-upgrade-push` target before accepting batch upgrades. The stock upgrade Job regenerates a cluster Deployment and node DaemonSet from `DeployConfig`. Keep Astribot installations with custom mounts, arguments or embedded topology on their own rollout path until an upgrade and rollback preserve the required configuration. The upstream Kubernetes workload path remains at preview maturity. Docker and Raw workload execution are unimplemented.

References

- [Upstream cluster upgrades](../admin-guide/upgrade.md)
- [Node management](../admin-guide/nodes.md)
- [Jobs](../user-guide/jobs.md)
- [Agent](../admin-guide/agent.md)
