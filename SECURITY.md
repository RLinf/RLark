# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in RLark, please report it privately to the maintainers at **security@rlinf.org**. Do not open a public issue.

We will respond within 5 business days to acknowledge receipt and provide an estimated timeline for a fix.

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| main    | :white_check_mark: (development) |
| v0.1.x  | :white_check_mark: |

## Security Best Practices

- **Certificates**: RLark uses X.509 mTLS and SSH certificates for authentication. Always keep your CA private keys secure and rotate certificates regularly.
- **Agent Access**: Data plane Agents authenticate via client certificates. Ensure each Agent has its own certificate with minimal permissions.
- **Network**: Cross-cluster Pod networking uses SSH tunnels. Consider using network policies to restrict traffic between Domains.
- **TLS Verification**: Production deployments should use valid TLS certificates with proper verification. Some development configurations may use `InsecureSkipVerify` — do not use these in production.