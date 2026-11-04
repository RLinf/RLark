# Migrate Cluster Delist State

RLark now uses `Cluster.spec.delisted` to represent that an administrator has delisted a cluster. Earlier releases used `Cluster.spec.deleted` for the same durable intent.

## Upgrade order

1. Deploy a transition CRD that accepts both `spec.deleted` and `spec.delisted`.
2. Run the migration script against the Kubernetes cluster hosting kcp before removing `spec.deleted` from the CRD:

```bash
KUBECONFIG=~/.kube/host.kubeconfig \
  ./hack/migrate-cluster-deleted-to-delisted.sh rlark-system kcp-0
```

3. Confirm the dry run reports no remaining resources:

```bash
KUBECONFIG=~/.kube/host.kubeconfig \
  ./hack/migrate-cluster-deleted-to-delisted.sh rlark-system kcp-0 --dry-run
```

4. Deploy the release that removes `spec.deleted` from the Cluster CRD and uses `spec.delisted` exclusively.

The script obtains the kcp administrator kubeconfig from `rlark-kcp-kubeconfig`, port-forwards the selected kcp Pod, then targets the kcp API rather than the host Kubernetes API. It migrates only Cluster resources where `spec.deleted=true`, sets `spec.delisted=true`, and removes the legacy field with a merge patch. It is idempotent: rerunning it after a successful migration makes no changes.

Do not apply a CRD that rejects or prunes `spec.deleted` before the migration has run, because the old field may be removed before its meaning is transferred to `spec.delisted`.
