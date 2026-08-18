# Quick Start

This guide walks you through setting up rlark locally and running your first training job.

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | >= 1.26.5 | Compile Go code |
| Docker | >= 24.0 | Run kcp and kind clusters |
| kind | >= 0.20 | Run local k8s data plane cluster |
| kubectl | >= 1.28 | Interact with clusters |
| jq | >= 1.6 | Parse JSON responses |

## 1. Build

```bash
git clone https://github.com/RLinf/RLark
cd RLark
# Linux: make build
# macOS: GOOS=darwin make build
make build
```

After building, the `apps/rlark/bin/` directory contains:

```
apps/rlark/bin/
├── server                # Control plane Server
├── gateway               # API Gateway
├── controller-manager    # Control plane controllers
├── agent                 # Data plane Agent
├── network-sidecar       # Pod network Sidecar
└── rlarkadm              # Deployment CLI
```

## 2. Local Development Environment

rlark supports quick local development environment setup via Docker Compose, including kcp cluster, database, and necessary runtime components.

### 2.1 Create runtime directory

```bash
mkdir -p ~/.rlark/certs
```

### 2.2 Start Control Plane

```bash
# Start kcp and PostgreSQL via Docker Compose
docker compose -f apps/rlark/docs/examples/docker-compose.yml up -d

# Wait for services to be ready (~30 seconds)
# Check status
docker compose -f apps/rlark/docs/examples/docker-compose.yml ps

# Extract admin kubeconfig from kcp container
docker cp kcp:/.kcp/admin.kubeconfig ~/.rlark/admin.kubeconfig

# Replace Docker internal IP with localhost and skip TLS verification
# (kcp's TLS certificate is for the Docker IP, not localhost)
python3 -c "
import yaml, re
with open('$HOME/.rlark/admin.kubeconfig') as f:
    config = yaml.safe_load(f)
for cluster in config.get('clusters', []):
    cluster['cluster']['server'] = re.sub(
        r'https://[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+:',
        'https://localhost:', cluster['cluster']['server'])
    cluster['cluster']['insecure-skip-tls-verify'] = True
    cluster['cluster'].pop('certificate-authority-data', None)
with open('$HOME/.rlark/admin.kubeconfig', 'w') as f:
    yaml.dump(config, f)
"

# Install CRDs into kcp (required before starting components)
kubectl --kubeconfig ~/.rlark/admin.kubeconfig apply -f api/config/crd/bases/
```

Components include:
- **kcp**: API Server (control plane cluster)
- **postgresql**: rlark operational database

### 2.3 Create Data Plane Kind Cluster

```bash
# Create kind cluster (if not already created)
kind create cluster --name rlark-data

# Export kubeconfig
kind get kubeconfig --name rlark-data > ~/.rlark/kind-kubeconfig
```

### 2.4 Start Control Plane Components

Open three terminals and start Server, Controller-Manager, and Gateway:

```bash
# Terminal 1: Start Server
./apps/rlark/bin/server \
  --kubeconfig ~/.rlark/admin.kubeconfig \
  --https-port 8443 \
  --ssh-port 2222 \
  --auto-sign-tls-ca-cert \
  --db-config apps/rlark/docs/examples/db-config.yaml

# Terminal 2: Start Controller-Manager
./apps/rlark/bin/controller-manager \
  --kubeconfig ~/.rlark/admin.kubeconfig \
  --server-address https://localhost:8443 \
  --leader-elect=false \
  --metrics-bind-address :0 \
  --db-config apps/rlark/docs/examples/db-config.yaml

# Terminal 3: Start Gateway
./apps/rlark/bin/gateway \
  --kubeconfig ~/.rlark/admin.kubeconfig \
  --addr :8080 \
  --server-address https://localhost:8443 \
  --db-config apps/rlark/docs/examples/db-config.yaml
```

### 2.5 Generate Agent Certificate

The Agent requires a client certificate to authenticate with the control plane. Request one via the Gateway API:

```bash
# Generate agent certificate (replace "my-cluster" with your cluster name)
RESP=$(curl -s -X POST "http://localhost:8080/api/v1/certificates/agent" \
  -H "Content-Type: application/json" \
  -d '{"cluster_id": "my-cluster"}')
echo "$RESP" | jq -r .ca_cert > ~/.rlark/certs/ca-cert.pem
echo "$RESP" | jq -r .agent_cert > ~/.rlark/certs/cert.pem
echo "$RESP" | jq -r .agent_key > ~/.rlark/certs/key.pem
```

This writes three files to `~/.rlark/certs/`:
- `ca-cert.pem` — CA certificate for verifying the Server
- `cert.pem` — Agent client certificate (X.509, signed by the control plane CA)
- `key.pem` — Agent private key

### 2.6 Start Data Plane Agent

```bash
./apps/rlark/bin/agent \
  --kubeconfig ~/.rlark/kind-kubeconfig \
  --server-address https://localhost:8443 \
  --client-cert ~/.rlark/certs/cert.pem \
  --client-key ~/.rlark/certs/key.pem \
  --ca-cert ~/.rlark/certs/ca-cert.pem \
  --mode both \
  --rlark-server-ssh-address localhost:2222
```

## 3. Verify Environment

### 3.1 Check Agent Registration

After Agent starts, it will automatically register nodes in the control plane:

```bash
# View registered Nodes
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes?namespace=default" | jq .
```

### 3.2 Check Control Plane

```bash
# Verify the API is working (should return nodes list)
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/nodes"
```

## 4. Create Your First Training Job

### 4.1 Create a Domain

```bash
curl -X POST "http://localhost:8080/api/v1/rlinf.io/v1alpha1/domains" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Domain",
    "metadata": { "name": "my-first-domain" },
    "spec": { "cidr": "10.0.1.0/24" }
  }'
```

### 4.2 Create a Job

```bash
curl -X POST "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs" \
  -H "Content-Type: application/json" \
  -d '{
    "apiVersion": "rlinf.io/v1alpha1",
    "kind": "Job",
    "metadata": { "name": "hello-world" },
    "spec": {
      "domain": "my-first-domain",
      "tasks": [
        {
          "name": "trainer",
          "head": true,
          "role": "Actor",
          "agentType": "Kubernetes",
          "nodeSelector": { "rlark.io/cluster-id": "my-cluster" },
          "kubernetes": {
            "workload": {
              "kind": "Deployment",
              "replicas": 1,
              "template": {
                "spec": {
                  "restartPolicy": "Always",
                  "containers": [
                    {
                      "name": "trainer",
                      "image": "busybox:latest",
                      "command": ["sh", "-c", "echo Hello from rlark! && sleep 3600"],
                      "resources": {
                        "limits": { "cpu": "100m", "memory": "128Mi" }
                      }
                    }
                  ]
                }
              }
            }
          }
        }
      ]
    }
  }'
```

### 4.3 Check Job Status

```bash
# Check Job status
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/hello-world" | jq '.status'

# Check Task status
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/tasks?namespace=default&labelSelector=rlinf.io/job=hello-world" | jq '.items[].status'

# Check Pod logs
curl "http://localhost:8080/api/v1/rlinf.io/v1alpha1/jobs/hello-world/logs" | jq .
```

### 4.4 Verify in Kind Cluster

```bash
# Data plane should show the Deployment
kubectl --kubeconfig ~/.rlark/kind-kubeconfig get deployment -A

# View Pods
kubectl --kubeconfig ~/.rlark/kind-kubeconfig get pods -A
```

## 5. Use Web UI

```bash
# Start frontend dev server
cd apps/rlark-ui && npm install && npm run dev
```

Open `http://localhost:5173` in your browser to see:
- Dashboard: System overview
- Nodes: Node list and resource usage
- Jobs: Create and manage training jobs
- Workflows: DAG workflow orchestration

## 6. Cleanup

```bash
# Stop all components
docker compose -f apps/rlark/docs/examples/docker-compose.yml down

# Delete kind cluster
kind delete cluster --name rlark-data

# Clean up kcp data
docker compose -f apps/rlark/docs/examples/docker-compose.yml down -v

# Remove runtime files
rm -rf ~/.rlark
```

## 7. Next Steps

- Read [Core Concepts](concepts.md) to understand the resource model
- Read [Architecture](architecture.md) to understand implementation principles
- Read [API Examples](../api/examples.md) for complete API usage
- Read [Deployment Guide](deployment.md) for production deployment