# Astribot IAM Deployment and Operations

Astribot IAM (`astribot-iam`) is the identity layer of the RLark console: human accounts, sessions, and audit live in the control plane's PostgreSQL under the `astribot_iam_*` tables, passwords are stored only as bcrypt hashes. With Scheme B, the install-time native credentials in `rlark-ui-auth` are not used for human sign-in or service-account provisioning. The Astribot release disables Gateway native username/password login with `--disable-native-login`; standalone Gateway deployments retain the default native login behavior.

## Current shape: IAM is the front door

Since 2026-11-25 the console's nginx (`astribot/release/rlark-ui.nginx.conf`) asks IAM's `/internal/admit` for page requests and `/internal/authorize` for requests matched by its `/api/` location. People sign in at `/login`:

- `/login` is a static page in the console image (`astribot/release/login.html`). On desktop, the **Sign in** form sits beside the product name and robot illustration in a centered panel. On narrow screens, the product panel sits above the form. Use **Show** or **Hide** to check the password. A failed attempt shows a localized message and enables **Sign in** again. A page asked for without a session is answered `302 /login?next=<address>`. After signing in from the home page or directly at `/login`, administrators land at `/admin` and other roles land at `/`. Sign-in preserves deep links, with `/admin` links reserved for administrators. `next` is read as the rest of the query, so it has to be the last parameter. Opening `/login` also clears the browser's IAM session cookie, including when IAM did not confirm revocation during sign-out; the page is never cached, so this holds on a revisit.
- The console image built by `astribot/release/rlark-ui.Dockerfile` sets `VITE_IAM_PORTAL=1`, which turns on the portal guard (`apps/rlark-ui/src/astribot/iam/portal.ts`). The guard:
  - starts every tab from the cookie's session (`GET /api/iam/session`): a new tab, a bookmark, or a tab that still holds an older token enters without the password again, and a tab that holds a token renders with it when IAM cannot be reached;
  - takes the console's own "Sign out" (the avatar menu, on every page): it revokes the session (`POST /api/iam/logout`), returns to `/login`, and the other tabs of the origin leave as well;
  - takes the console to `/login` when a call answers 401 (the session expired, or was ended by a password change, a reset, or a disable), and checks the session again when the browser restores a tab from its back/forward cache;
  - never shows the native RLark sign-in page.
- A console built another way (the stock `apps/rlark-ui` image) does not set the variable and keeps the native sign-in.
- A business call reaches the gateway with the role's service-account token, which nginx injects; the token the browser holds authorizes nothing by itself.
- `/api/iam/*` goes to IAM directly. The release Nginx returns HTTP 403 JSON for `POST /api/v1/auth/login`, and the Gateway independently rejects it when `--disable-native-login` is set.

## Components and files

| Component | Location |
| --- | --- |
| IAM service (Go) | `apps/rlark/astribot/iam/`, entrypoint `apps/rlark/astribot/cmd/astribot-iam` |
| Portal guard and console page | `apps/rlark-ui/src/astribot/iam/` (`portal.ts`; the admin console's "Identity & Access" page, `/admin/iam`) |
| Sign-in page | `astribot/release/login.html` (copied into the console image) |
| Release wiring | `astribot/release/rlark.Dockerfile`, `astribot/release/astribot-iam.Dockerfile`, `astribot/release/rlark-ui.Dockerfile`, `astribot/release/images`, `astribot/release/control-plane/rlark.compose.yaml`, `astribot/release/rlark-ui.nginx.conf` |
| Service-account provisioning | `astribot/release/iam-provision.sh` (bootstraps service-account Secret hashes through Kubernetes; plaintext tokens are written only to `${R}/iam/upstream-tokens.json`, mode 0600) |

In compose, `astribot-iam` listens on 8081 inside the internal network only, with no host port; `rlark-ui` forwards via `ASTRIBOT_IAM_URL`. The service's image is `ASTRIBOT_IAM_IMAGE`, falling back to the release's `rlark-server` image, which carries the same binary. The gateway's `19000` stays as is (Fleet depends on it); tighten its sources later with a network policy.

Gateway stores only the SHA-256 hex digest in the management-namespace Secret `rlark-service-account-<name>` under `data.token-sha256`; the bearer token is `rlsa.<name>.<random>` and exists in plaintext only in the mode-0600 IAM token file.
Set `RLARK_MANAGEMENT_NAMESPACE` to the namespace used by the Gateway's `--kube-namespace` setting when it is not `default`; the provisioning script must target the same namespace.

## First deployment

Before applying a release that records workload submitters, run `sudo env R=/home/cailiang/.local/share/rlark-runtime astribot/release/iam-provision.sh --submission-keys-only` on the management host. This step is safe to repeat and reuses the existing key pair. Back up `${R}/iam/submission-private.pem` (mode 0600) with the IAM upstream credentials. The Gateway mounts only `submission-public.pem`.

For Job and Workflow creation, IAM signs a short-lived assertion with the user ID and username. The Gateway verifies its signature, expiry, route, and service account, then records `rlark.io/submitted-by` and `rlark.io/submitter-id`. Gateway writes preserve these annotations on edits. During a mixed-version rollout, a request from a shared account without an assertion can still submit jobs, with no human attribution. Roll Gateway, the controller manager, IAM, and the console together, then verify that two different users and a Workflow-generated job have the expected submitters.

1. Ship a normal release (`astribot/release/build.sh` + `rlarkctl release apply`). The apply console stage starts `astribot-iam` and then `rlark-ui`; first startup creates the tables (`astribot_iam_users/sessions/audit/schema_migrations`, guarded by an advisory lock) and bootstraps the two default accounts.
2. Provision the upstream service accounts on the management machine as root (the script writes the token hashes directly to management-namespace Secrets without using native login; rerun with `--rotate-upstream-tokens` to rotate):

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
   curl -s -o /dev/null -w '%{http_code}\n' -X POST http://127.0.0.1/api/v1/auth/login   # 403 native_login_disabled
   ```

   The service's own `/healthz` is on the container network (`docker exec rlark-ui wget -qO- http://astribot-iam:8081/healthz`). Then open `http://<console>/`: the sign-in page; **Identity & Access** (`/admin/iam`) is the administrator's, signed in with `admin / astribot`.

## Default accounts and first password change

| Account | Initial password | Role |
| --- | --- | --- |
| `admin` | `astribot` | admin |
| `user` | `123` | operator |

Both defaults are weak: change them right after the first sign-in under **My profile → Change password** (new passwords are at least 8 characters). An administrator resetting another user's password needs no old password, and the reset revokes all of that user's sessions.

## Day-2 operations

The Astribot console sidebar displays the Astribot and RLark logos in both light and dark themes. The compact sidebar shows the Astribot mark.

- **Users and roles**: administrators create (admin/operator/viewer), edit, disable/enable, delete, and reset users under "Identity & Access → Users". Operators read and write the business plane only, viewers read only; the IAM management endpoints are admin-only.
- **Create user** opens a side panel above the console header. Use the close button, **Cancel**, or Escape to return to the user list.
- Use the **Role** and **Status** dropdowns above the user list to filter accounts. Each dropdown has a down arrow on the right in both themes.
- **Audit**: the "Audit" page records successful and failed sign-ins, sign-outs, user create/edit/delete, password resets and changes, and authorization denials; filter by actor, action, result, and time.
- **Sessions**: sessions last 7 days; a password change, reset, disable, or revoke invalidates the affected sessions immediately. Established WebSocket terminals are untouched until their next handshake.
- **Rolling IAM, the sign-in page, or the portal guard alone**: `astribot-iam` also builds as a standalone image (`astribot/release/astribot-iam.Dockerfile`; the service is one binary), and the console image builds from `astribot/release/rlark-ui.Dockerfile`. Publish each into the control plane's pull registry by digest, name it in the release's `images` (`astribot-iam`, `rlark-ui`), and run `rlarkctl release apply`: the console stage makes `astribot-iam` anew and then `rlark-ui` (nginx resolves the address at startup). Rolling IAM signs nobody out: the sessions are in PostgreSQL.
- **Upstream token rotation**: run `iam-provision.sh --rotate-upstream-tokens`; it atomically replaces the token file after applying new SHA-256 hashes to the service-account Secrets. Restart `astribot-iam` so it reads the new credentials. IAM stores only hashes of its own sessions; the upstream token file is the only plaintext on disk — keep it at 0600 and include it in backups or rotate per above.
- **Recovering the console after a host reboot** — If the home page responds with 500 and `/api/iam/session` with 502, check that IAM and Gateway are running. Restart `rlark-ui` with `docker restart rlark-ui` so nginx resolves their current container addresses. Without a cookie, the home page should redirect to `/login` and the session endpoint should respond with 401.

## Backup and recovery

The control plane PostgreSQL's regular backups already cover the three `astribot_iam_*` tables; restoring the database restores every account and session (sessions expire naturally by `expires_at`). `${R}/iam/upstream-tokens.json` is not in the database — back it up separately or rotate as above.

## Rollback

`rlarkctl release apply --back` walks the stages backwards and takes `astribot-iam` and `rlark-ui` back to the previous versions together; the `astribot_iam_*` tables are kept. A rollback to a release that predates Scheme B may re-enable native Gateway login, so keep the release boundary consistent with the intended IAM-only deployment.

## Known limitations

- Every human account is managed by IAM, and the native sign-in page is not reachable in the Astribot release.
- IAM admits `/admin` and `/admin/*` only for admins. Nginx redirects a signed-in non-admin page request to `/` while API denials remain JSON 403 responses.
- `/internal/authorize` trusts callers that can reach the IAM container network. A caller there with a valid session can obtain that role's upstream token, so IAM must not publish a host port.
- If IAM does not confirm session revocation during sign-out, the browser still exits and `/login` clears its cookie, while the server-side session remains valid until expiry.
- With IAM down the console's pages and business calls answer 5xx from nginx; the sign-in page is static and still loads, and signing in fails until IAM is back.
- IAM does not change RLark's tenancy/ownership model; users of the same role see the same business resources as before.
- A principal holding a gateway service-account token, or one that can reach 19000 directly, still bypasses IAM (the machine API stays for Fleet by plan).
- No SSO/OIDC yet; failed sign-ins back off per account+IP (5 failures lock for 30 seconds).
