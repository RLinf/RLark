# SSH Keys

Use this guide to register a public key for RLark SSH bastion authentication and, optionally, select it when creating a Job.

> **Screenshot note:** Screenshots are from an example environment. Resource names and data are illustrative; your environment will differ.

![SSH Key Management](../images/ui/ssh-key-ui.png)

## Task 1: Add a Public Key

1. Generate a dedicated key pair for RLark. Never upload the private key.
2. Open **SSH Keys** in the platform console and choose **Add Key**.
3. Enter the SSH username. It must match the username used in the SSH command.
4. Paste one OpenSSH public key, such as `ssh-ed25519` or `ssh-rsa`.
5. Choose **Add** and confirm that the key appears in the list.

RLark validates the public-key format and requires both the SSH username and public key content to be globally unique. The form shows an inline error before upload when either value already exists. The page also shows the configured jump host command when the Server SSH address is available.

## Task 2: Select a Key for a Job

1. Open **Jobs** and start creating a Job.
2. In **Common Config**, choose an uploaded key under **SSH Public Key Injection**.
3. Review the YAML preview and confirm that every applicable Task template contains this Environment declaration:

   ```yaml
   environments:
     - type: rlinf.io/ssh
       version: v1
       parameters:
         authorizedKeys:
           - ssh-ed25519 AAAA...
   ```
4. Submit the Job.

Each selected public key is written into the applicable Task template's `rlinf.io/ssh/v1` Environment. Registering or changing a record on the SSH Keys page alone does not modify existing Jobs or Tasks. Job details display multiple configured keys in a scrollable list.

For built-in Ray-on-Kubernetes workloads, the Agent stores the Task's selected public key in a Task-incarnation-owned Secret, mounts its `authorized_key` item read-only at `/var/run/rlark/ssh/authorized_key` in each main container, and starts `rlark-tools sshd` as an optional supervised service. The mount does not use `subPath`, and `sshd` reopens the file for every authentication attempt. An empty file disables workload key authentication. An SSH service failure can make the Pod unreachable over SSH, but does not stop the Task. This automatic service lifecycle does not apply to Docker, Raw, or arbitrary custom workload implementations.

## Task 3: Connect Through the Bastion

After an administrator configures the Server SSH endpoint, use the command displayed by the console or copy the Worker-specific command from Job or Node details. A typical Worker command is:

```bash
ssh -J <registered-user>@<bastion-host>:<port> root@<worker-name>
```

The outer SSH username must match the username attached to the registered key. The target container must provide its SSH service and accept the selected key.

The "Public Key Name" in the SSH key list is the registered SSH username, which may differ from your console login. Gateway and Server use the `rlark-ssh-keys` Secret in the same management namespace to store and verify public keys. Enabling PostgreSQL does not change this behavior; no manual database entry is required.

!!! warning "No Pod-level authorization"
    The current SSH server authenticates a username and registered public key, but it does not implement authorization that limits that user to particular Pods. Treat bastion access as trusted-user access; do not describe key registration as granting access to selected Pods only.

## Task 4: Delete or Rotate a Key

1. Add the replacement public key and select it in the Job configuration.
2. Verify access with the replacement private key.
3. Delete the old key from **SSH Keys**.
4. Ensure existing Jobs/Tasks have been updated or replaced so their workload SSH configuration no longer contains the old key.

Deleting a registered key prevents subsequent bastion authentication with that username/key pair. It does not edit an existing Job or Task whose SSH Environment already contains the old key. Rotate workload access by replacing or clearing `parameters.authorizedKeys`. The projected file update is eventually consistent with kubelet propagation; revocation is not guaranteed to be instantaneous.

## Security Notes

- Key records are keyed by the entered SSH username, not isolated by the current console session.
- Anyone holding a matching private key can authenticate as that SSH username.
- Never upload or place private keys in Job YAML, scripts, or environment variables.
- Do not copy `/var/run/rlark/ssh/authorized_key` into a durable file or configure the SSH service to cache its resolved path or an open file descriptor; doing so prevents rotation and revocation from taking effect.
- Verify the bastion host-key fingerprint before first use and rotate keys regularly.

!!! warning "Current security boundaries"
    RLark does not currently document a durable SSH authentication/connection audit-log location; do not rely on the platform as the system of record for SSH audits. Put the bastion behind external centralized logging and retention controls. Built-in SSO/OIDC integration is not available, so production deployments should enforce identity and access policy through an external SSO/OIDC-aware access proxy or bastion. SSH key records and bastion authorization are not tenant- or Pod-scoped; use separate deployments or separately controlled bastions for mutually untrusted tenants.

## API Equivalent

Use `GET` and `POST /api/v1/ssh-user-keys` and `DELETE /api/v1/ssh-user-keys/{index}?user={user}`. The index is zero-based within that user's registered keys. See [API Reference](../api/reference.md).
