# Astribot IAM Deployment and Operations

Astribot IAM (`astribot-iam`) is an independent identity-and-access layer above the RLark console: human accounts, sessions, and audit live in the control plane's PostgreSQL under the `astribot_iam_*` tables, passwords are stored only as bcrypt hashes, and the identity page no longer relies on the install-time random shared passwords kept in `rlark-ui-auth`. No RLark-native code was modified; everything lives under `astribot/` and the Astribot release files.

## Current shape (shadow launch)

This release is a **shadow launch**: nginx routes only `/api/iam/*` to `astribot-iam`; `/`, `/admin`, and the remaining `/api/*` still sign in and authorize through the native RLark gateway.

- The console's main entrance is still the native login page (the `rlark-ui-auth` admin/user).
- An administrator signs in natively, opens **Identity & Access** (`/admin/iam`), and signs in once more inside that page with an IAM account (below). The double sign-in is a known limitation of the shadow stage; it disappears once the front door switches (nginx `auth_request` + `/login`).
- `/internal/authorize` is ready but has no caller yet; until nginx wires `auth_request`, its verdicts do not affect production.

## Components and files

| Component | Location |
| --- | --- |
| IAM service (Go) | `apps/rlark/astribot/iam/`, entrypoint `apps/rlark/astribot/cmd/astribot-iam` |
| Console page | `apps/rlark-ui/src/astribot/iam/` (the admin console's "Identity & Access" page, `/admin/iam`) |
| Release wiring | `astribot/release/rlark.Dockerfile`, `astribot/release/images`, `astribot/release/control-plane/rlark.compose.yaml`, `astribot/release/rlark-ui.nginx.conf`, `astribot/release/iam-provision.sh` |
| Service-account provisioning | `astribot/release/iam-provision.sh` (creates the `astribot-iam-admin/operator/user` gateway service accounts; plaintext tokens are written only to `${R}/iam/upstream-tokens.json`, mode 0600) |

In compose, `astribot-iam` listens on 8081 inside the internal network only, with no host port; `rlark-ui` forwards via `ASTRIBOT_IAM_URL`. The gateway's `19000` stays as is (Fleet depends on it); tighten its sources later with a network policy.

## First deployment

1. Ship a normal release (`astribot/release/build.sh` + `rlarkctl release apply`). The apply console stage starts `astribot-iam` and then `rlark-ui`; first startup creates the tables (`astribot_iam_users/sessions/audit/schema_migrations`, guarded by an advisory lock) and bootstraps the two default accounts.
2. Provision the upstream service accounts on the management machine as root (signing in to the gateway needs them; the script is idempotent and exits when valid tokens exist):

   ```bash
   sudo env R=/home/cailiang/.local/share/rlark-runtime \
     astribot/release/iam-provision.sh
   ```

3. Restart IAM once so it reads the token file, together with the console (nginx resolves the address at startup):

   ```bash
   sudo docker compose -p rlark -f /var/lib/rlark/rlark.compose.yaml \
     --env-file /var/lib/rlark/control-plane.env up -d --no-deps astribot-iam rlark-ui
   ```

4. Verify: `curl -fsS http://127.0.0.1:80/api/iam/healthz` (through nginx); open `http://<console>/admin/iam` and sign in to the IAM page with `admin / astribot`.

## Default accounts and first password change

| Account | Initial password | Role |
| --- | --- | --- |
| `admin` | `astribot` | admin |
| `user` | `123` | operator |

Both defaults are weak: change them right after the first sign-in under **My profile → Change password** (new passwords are at least 8 characters). An administrator resetting another user's password needs no old password, and the reset revokes all of that user's sessions.

## Day-2 operations

- **Users and roles**: administrators create (admin/operator/viewer), edit, disable/enable, delete, and reset users under "Identity & Access → Users". Operators read and write the business plane only, viewers read only; the IAM management endpoints are admin-only.
- **Audit**: the "Audit" page records successful and failed sign-ins, sign-outs, user create/edit/delete, password resets and changes, and authorization denials; filter by actor, action, result, and time.
- **Sessions**: sessions last 7 days; a password change, reset, disable, or revoke invalidates the affected sessions immediately. Established WebSocket terminals are untouched until their next handshake.
- **Upstream token rotation**: delete or rotate `${R}/iam/upstream-tokens.json` by re-running `iam-provision.sh` (existing accounts with a matching role go through the rotation endpoint), then restart `astribot-iam`. IAM stores only hashes of its own sessions; the upstream token file is the only plaintext on disk — keep it at 0600 and include it in backups or rotate per above.

## Backup and recovery

The control plane PostgreSQL's regular backups already cover the three `astribot_iam_*` tables; restoring the database restores every account and session (sessions expire naturally by `expires_at`). `${R}/iam/upstream-tokens.json` is not in the database — back it up separately or rotate as above.

## Rollback

`rlarkctl release --back` walks the console stage backwards and takes `astribot-iam` and `rlark-ui` back to the previous versions together; the `astribot_iam_*` tables are kept, and after a rollback the native sign-in works as before.

## Known limitations

- The shadow stage signs in twice (native console + IAM page); the front-door switch removes this.
- The native console's "Sign out" clears only its own token, not the IAM session; signing out inside "Identity & Access" performs the IAM sign-out.
- IAM does not change RLark's tenancy/ownership model; users of the same role see the same business resources as before.
- A principal holding a gateway service-account token, or one that can reach 19000 directly, still bypasses IAM (the machine API stays for Fleet by plan).
- No SSO/OIDC yet; failed sign-ins back off per account+IP (5 failures lock for 30 seconds).
