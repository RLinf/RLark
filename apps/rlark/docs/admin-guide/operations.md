# Operations and Troubleshooting

## Health Check Flow

Check the system from control plane to data plane in this order:

1. **Service Health**: Verify all components are running
2. **Database and kcp**: Check database connectivity and kcp availability
3. **Gateway Authentication**: Verify Gateway API is accessible and accepting requests
4. **Agent Heartbeat**: Confirm agents are connected and reporting
5. **Resource Synchronization**: Verify nodes, resources, and labels are synced
6. **Kubernetes Events**: Check for errors and warnings
7. **Workload Logs**: Review Task and Worker logs
8. **SSH Tunnel Connectivity**: Verify cross-cluster networking

## Common Issues

### Control Plane

| Symptom | Check | Resolution |
|---------|-------|------------|
| Gateway returns 503 | `kubectl get pods -n rlark-system` | Restart unhealthy pods |
| Database connection errors | `kubectl logs -n rlark-system deploy/rlark-server` | Verify db-config.yaml credentials |
| kcp not responding | `kubectl get pods -n rlark-system -l app=kcp` | Check kcp pod logs, restart if needed |

### Agent

| Symptom | Check | Resolution |
|---------|-------|------------|
| Agent not connecting | `kubectl logs -n rlark-system deploy/rlark-agent` | Verify TLS certificates and server address |
| Agent heartbeat missing | Check network connectivity to control plane | Verify firewall rules, DNS resolution |
| Nodes not appearing | `kubectl get nodes -l rlark.io/cluster-id` | Verify node labels are applied |

### Jobs

| Symptom | Check | Resolution |
|---------|-------|------------|
| RLark Job stuck in Pending | `kubectl describe jobs.rlinf.io <name>` | Check nodeSelector matches available nodes |
| Task not created | Check controller-manager logs | Verify Job namespace matches node namespace |
| Worker fails to start | `kubectl describe pod <worker-name>` | Check image pull, resource availability |
| Cross-cluster network fails | Check Domain CRD and SSH tunnels | Verify DomainPeer resources, restart network-sidecar |

### Storage

| Symptom | Check | Resolution |
|---------|-------|------------|
| PVC stuck in Pending | `kubectl describe pvc <name>` | Check StorageClass exists and provisioner is running |
| Mount fails | Check hostPath exists on node | Verify path permissions and node availability |
| Object storage unreachable | Check storage provider configuration | Verify endpoint, credentials, and network |

### Diagnostics Collection

When collecting diagnostics for issue reports:

```bash
# Component versions
rlark-server --version
rlark-agent --version

# Pod status
kubectl get pods -n rlark-system -o wide

# Recent events
kubectl get events -n rlark-system --sort-by='.lastTimestamp' | tail -50

# Component logs
kubectl logs -n rlark-system deploy/rlark-server --tail=100
kubectl logs -n rlark-system deploy/rlark-agent --tail=100
kubectl logs -n rlark-system deploy/rlark-controller-manager --tail=100

# Resource status
kubectl get nodes -o wide
kubectl get jobs.rlinf.io -A
kubectl get domains -A
```

!!! warning "Diagnostic data safety"
    Record component versions and timestamps when collecting diagnostics. Never include tokens, private keys, or generated credentials in issue reports. Redact passwords from configuration files before sharing.