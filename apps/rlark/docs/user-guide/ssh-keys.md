# SSH Keys

Use this guide to register a public key for RLark SSH bastion authentication and, optionally, select it when creating a Job.

![SSH Key Management](../images/ui/ssh-key-ui.png)

## Task 1: Add a Public Key

1. Generate a dedicated key pair for RLark. Never upload the private key.
2. Open **SSH Keys** in the platform console and choose **Add Key**.
3. Enter the SSH username. It must match the username used in the SSH command.
4. Paste one OpenSSH public key, such as `ssh-ed25519` or `ssh-rsa`.
5. Choose **Add** and confirm that the key appears in the list.

RLark validates the public-key format and rejects duplicate keys. The page also shows the configured jump host command when the Server SSH address is available.

## Task 2: Select a Key for a Job

1. Open **Jobs** and start creating a Job.
2. In **Common Config**, choose an uploaded key under **SSH Public Key Injection**.
3. Review the YAML preview and confirm that `spec.sshPublicKey` contains the selected public key.
4. Submit the Job.

This selection writes one public key into the Job configuration for workload injection. Registering a key on the SSH Keys page alone does not modify existing Jobs or Pods.

## Task 3: Connect Through the Bastion

After an administrator configures the Server SSH endpoint, use the command displayed by the console or copy the Worker-specific command from Job or Node details. A typical Worker command is:

```bash
ssh -J <ssh-user>@<bastion-host>:<port> root@<worker-name>
```

The outer SSH username must match the username attached to the registered key. The target container must provide its SSH service and accept the selected key.

!!! warning "No Pod-level authorization"
    The current SSH server authenticates a username and registered public key, but it does not implement authorization that limits that user to particular Pods. Treat bastion access as trusted-user access; do not describe key registration as granting access to selected Pods only.

## Task 4: Delete or Rotate a Key

1. Add the replacement public key and use it for newly created Jobs.
2. Verify access with the replacement private key.
3. Delete the old key from **SSH Keys**.
4. Recreate or separately update workloads that already contain the old key.

Deleting a registered key prevents subsequent bastion authentication with that username/key pair. It does not remove a public key already copied into a running workload.

## Security Notes

- Key records are keyed by the entered SSH username, not isolated by the current console session.
- Anyone holding a matching private key can authenticate as that SSH username.
- Never upload or place private keys in Job YAML, scripts, or environment variables.
- Verify the bastion host-key fingerprint before first use and rotate keys regularly.

!!! warning "Current security boundaries"
    RLark does not currently document a durable SSH authentication/connection audit-log location; do not rely on the platform as the system of record for SSH audits. Put the bastion behind external centralized logging and retention controls. Built-in SSO/OIDC integration is not available, so production deployments should enforce identity and access policy through an external SSO/OIDC-aware access proxy or bastion. SSH key records and bastion authorization are not tenant- or Pod-scoped; use separate deployments or separately controlled bastions for mutually untrusted tenants.

## API Equivalent

Use `GET` and `POST /api/v1/ssh-user-keys` and `DELETE /api/v1/ssh-user-keys/{index}?user={user}`. See [API Reference](../api/reference.md).
