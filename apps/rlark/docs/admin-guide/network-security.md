# Networking and Security

## Managing Network Domains

A Domain groups virtual addresses and scopes cross-cluster forwarding. Each Domain has a CIDR range for IP allocation. It is not an isolation boundary for the underlying infrastructure or ordinary Kubernetes networking.

### Creating a Domain

![Domain Management](../images/ui/domain-ui.png)

- Administrator Console → Domain Management → Create Domain
- Enter a unique name and CIDR range (e.g., `10.200.0.0/24`)
- The Domain CRD is created in kcp
- DomainPeer resources are automatically created for cross-cluster connectivity

### Inspecting IP Allocations
- Domain details show allocated IPs and their associated Pods
- Verify IP ranges don't overlap between Domains
- Cross-cluster Pods communicate via Domain IPs through SSH tunnels

### Deleting a Domain
- Verify no active Jobs reference the Domain
- Delete the Domain CRD
- DomainPeer resources are garbage collected

### Using the API
```bash
# Create Domain
kubectl apply -f domain.yaml

# List Domains
kubectl get domains -A

# Delete Domain
kubectl delete domain <name>
```

## Cross-Cluster Network Architecture

```
Client Pod (cluster-B)                    Server Pod (cluster-A)
  ├── wget → Domain IP (10.200.0.x)        ├── nc -l -p 8000
  ├── gVisor netstack intercepts           │
  ├── TUN device → NodeServer socket       │
  └── NodeServer → SSH tunnel → ──────────→ Proxy → localhost:8000
```

## Security Best Practices

- Enforce security-zone isolation with infrastructure controls such as Kubernetes NetworkPolicy; do not rely on Domains alone
- Rotate TLS certificates periodically via `rlarkadm`
- Review DomainPeer resources for unexpected cross-cluster connections
- SSH keys for Worker access should be managed through the platform, not directly on nodes
- Control-plane certificates are auto-generated but should be monitored for expiration