# CLI Reference

RLark provides the following command-line tools:

## rlarkadm

Deployment tool for installing and uninstalling RLark control plane and data plane components.

### rlarkadm install

Install RLark components.

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--install-conf` | `-f` | string | `""` | Install configuration file path (required) |

**Example:**
```bash
rlarkadm install -f deploy-control-plane.yaml
```

### rlarkadm uninstall

Uninstall RLark components.

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--uninstall-conf` | `-f` | string | `""` | Uninstall configuration file path (required) |
| `--purge` | | bool | `false` | Also delete namespace and data directories |
| `--yes` | `-y` | bool | `false` | Skip confirmation prompt |

!!! warning "`--purge` is irreversible"
    Using `--purge` permanently deletes namespaces and all associated data.

**Example:**
```bash
rlarkadm uninstall -f deploy-control-plane.yaml --purge -y
```

### Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--log-level` | string | `info` | Log level: debug, info, warn, error |

## rlarkctl (rlark-server-cli)

Server CLI tool for certificate management and proxy access.

### Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server-address` | string | `https://localhost:8443` | Server address |
| `--server-hostname` | string | `""` | Expected server TLS hostname |
| `--client-cert` | string | `""` | Client TLS certificate path |
| `--client-key` | string | `""` | Client TLS private key path |
| `--ca-cert` | string | `""` | CA certificate path |
| `--insecure-skip-tls-verify` | bool | `false` | Skip TLS certificate verification |

### rlarkctl sign

Sign an agent certificate.

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--role` | `-r` | string | `agent` | Certificate role: admin, peer, agent |
| `--client-id` | `-c` | string | `example-client-id` | Client ID for agent role |
| `--output` | `-o` | string | `""` | Output directory for cert and key |

**Example:**
```bash
rlarkctl sign \
  --role=agent \
  --client-id=agent-my-cluster-1 \
  --output=/tmp/agent-certs
```

### rlarkctl revoke

!!! warning "Not implemented"
    This command calls the Gateway certificate-revocation endpoint, whose handler is not implemented. Do not use it as an operational revocation mechanism.

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--cert-type` | `-t` | string | `""` | Certificate type: x509, ssh (required) |
| `--serial-number` | `-s` | string | `""` | Certificate serial number (required) |
| `--subject-key-id` | `-k` | string | `""` | Certificate Subject Key ID (required) |
| `--reason` | `-r` | string | `""` | Revocation reason (optional) |

**Example:**
```bash
rlarkctl revoke \
  --cert-type=x509 \
  --serial-number=12345 \
  --subject-key-id=abc:def:123
```

### rlarkctl proxy-curl

Send HTTP requests through the server proxy endpoint.

**Example:**
```bash
rlarkctl proxy-curl https://internal-service:8080/api/status
```