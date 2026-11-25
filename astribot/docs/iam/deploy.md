# Astribot IAM Deployment and Operations

Astribot IAM (`astribot-iam`) is the identity layer of the RLark console: human accounts, sessions, and audit live in the control plane's PostgreSQL under the `astribot_iam_*` tables, passwords are stored only as bcrypt hashes, and nobody signs in with the install-time random shared passwords kept in `rlark-ui-auth` (they only serve to provision the gateway service accounts, below). No RLark-native code was modified; everything lives under `astribot/`, `apps/rlark/astribot/`, and `apps/rlark-ui/src/astribot/`.

## Current shape: IAM is the front door

Since 2026-11-25 the console's nginx (`astribot/release/rlark-ui.nginx.conf`) admits every page and every business `/api/*` call only through IAM (`auth_request` → `/internal/authorize`), and people sign in at `/login`:

- `/login` is a static page in the console image (`astribot/release/login.html`). A page asked for without a session is answered `302 /login?next=<address>`; after signing in the browser returns to that address. An address under `/admin` is returned to only by the admin role. `next` is read as the rest of the query, so it has to be the last parameter.
- The console image built by `astribot/release/rlark-ui.Dockerfile` sets `VITE_IAM_PORTAL=1`, which turns on the portal guard (`apps/rlark-ui/src/astribot/iam/portal.ts`). The guard:
  - restores the session in a tab that has the cookie but not the token (a new tab, a bookmark) through `GET /api/iam/session`, instead of asking for the password again;
  - takes the console's own "Sign out" (the avatar menu, on every page): it revokes the session (`POST /api/iam/logout`) and returns to `/login`;
  - takes the console to `/login` when a call answers 401 (the session expired, or was ended by a password change, a reset, or a disable);
  - never shows the native RLark sign-in page.
- A console built another way (the stock `apps/rlark-ui` image) does not set the variable and keeps the native sign-in.
- A business call reaches the gateway with the role's service-account token, which nginx injects; the token the browser holds authorizes nothing by itself.
- `/api/iam/*` goes to IAM directly. `POST /api/v1/auth/login` still goes to the gateway without the session check (the native exchange); the native accounts no longer open the console, whose pages need an IAM session.

## Components and files

| Component | Location |
| --- | --- |
| IAM service (Go) | `apps/rlark/astribot/iam/`, entrypoint `apps/rlark/astribot/cmd/astribot-iam` |
| Portal guard and console page | `apps/rlark-ui/src/astribot/iam/` (`portal.ts`; the admin console's "Identity & Access" page, `/admin/iam`) |
| Sign-in page | `astribot/release/login.html` (copied into the console image) |
| Release wiring | `astribot/release/rlark.Dockerfile`, `astribot/release/astribot-iam.Dockerfile`, `astribot/release/rlark-ui.Dockerfile`, `astribot/release/images`, `astribot/release/control-plane/rlark.compose.yaml`, `astribot/release/rlark-ui.nginx.conf` |
| Service-account provisioning | `astribot/release/iam-provision.sh` (creates the `astribot-iam-admin/operator/user` gateway service accounts; plaintext tokens are written only to `${R}/iam/upstream-tokens.json`, mode 0600) |

In compose, `astribot-iam` listens on 8081 inside the internal network only, with no host port; `rlark-ui` forwards via `ASTRIBOT_IAM_URL`. The service's image is `ASTRIBOT_IAM_IMAGE`, falling back to the release's `rlark-server` image, which carries the same binary. The gateway's `19000` stays as is (Fleet depends on it); tighten its sources later with a network policy.

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

4. Verify from the management machine: a page without a session is sent to the portal, and signing in answers.

   ```bash
   curl -s -o /dev/null -D - http://127.0.0.1/ | grep -iE '^(HTTP|location)'   # 302, Location: /login?next=/
   curl -fsS -X POST -H 'Content-Type: application/json' \
     -d '{"username":"admin","password":"astribot"}' -o /dev/null http://127.0.0.1/api/iam/login
   ```

   The service's own `/healthz` is on the container network (`docker exec rlark-ui wget -qO- http://astribot-iam:8081/healthz`). Then open `http://<console>/`: the sign-in page; **Identity & Access** (`/admin/iam`) is the administrator's, signed in with `admin / astribot`.

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
- **Rolling IAM, the sign-in page, or the portal guard alone**: `astribot-iam` also builds as a standalone image (`astribot/release/astribot-iam.Dockerfile`; the service is one binary), and the console image builds from `astribot/release/rlark-ui.Dockerfile`. Publish each into the control plane's pull registry by digest, name it in the release's `images` (`astribot-iam`, `rlark-ui`), and run `rlarkctl release apply`: the console stage makes `astribot-iam` anew and then `rlark-ui` (nginx resolves the address at startup). Rolling IAM signs nobody out: the sessions are in PostgreSQL.
- **Upstream token rotation**: delete or rotate `${R}/iam/upstream-tokens.json` by re-running `iam-provision.sh` (existing accounts with a matching role go through the rotation endpoint), then restart `astribot-iam`. IAM stores only hashes of its own sessions; the upstream token file is the only plaintext on disk — keep it at 0600 and include it in backups or rotate per above.

## Backup and recovery

The control plane PostgreSQL's regular backups already cover the three `astribot_iam_*` tables; restoring the database restores every account and session (sessions expire naturally by `expires_at`). `${R}/iam/upstream-tokens.json` is not in the database — back it up separately or rotate as above.

## Rollback

`rlarkctl release apply --back` walks the stages backwards and takes `astribot-iam` and `rlark-ui` back to the previous versions together; the `astribot_iam_*` tables are kept. A console from before the front door switched signs in natively again (the `rlark-ui-auth` accounts).

## Known limitations

- Every human account is IAM's: the native sign-in page is not reachable, and the native admin/user accounts only serve to provision the gateway service accounts.
- With IAM down the console's pages and business calls answer 5xx from nginx; the sign-in page is static and still loads, and signing in fails until IAM is back.
- IAM does not change RLark's tenancy/ownership model; users of the same role see the same business resources as before.
- A principal holding a gateway service-account token, or one that can reach 19000 directly, still bypasses IAM (the machine API stays for Fleet by plan).
- No SSO/OIDC yet; failed sign-ins back off per account+IP (5 failures lock for 30 seconds).
