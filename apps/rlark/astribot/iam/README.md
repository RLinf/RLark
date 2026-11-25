# Astribot IAM Backend

`astribot-iam` is the identity service behind the console's front door (`astribot/docs/iam/deploy.md`). It stores human users, opaque sessions, and audit events in PostgreSQL. It does not provision gateway service accounts or serve the console UI.

## Runtime

Run `astribot/cmd/astribot-iam` with `--db-config` pointing at the existing RLark PostgreSQL configuration. Startup applies the versioned `astribot_iam_*` migrations under an advisory transaction lock and idempotently bootstraps `admin` / `astribot` (`admin`, `u_admin`) and `user` / `123` (`operator`, `u_user`). Bootstrap passwords are bcrypt-hashed at cost 12. Change them after first login. User-created and reset passwords require at least eight characters and fit bcrypt's 72-byte input limit.

The default listener is `:8081`. `--session-ttl` defaults to `168h`. Login issues a 32-byte opaque token, stores only its SHA-256 digest, and sets the HttpOnly `astribot_session` cookie with SameSite=Lax. Configure `--cookie-secure=true` when TLS terminates before the service.

`--upstream-tokens` reads a JSON object with `admin`, `operator`, and `user` string values. The file must be mode 0600 or stricter. Without it sign-in and IAM management keep working, while `/internal/authorize` answers 503 to business calls. The service never reads `rlark-ui-auth`.

## HTTP API

- `POST /api/iam/login` and `POST /api/v1/auth/login` accept `{ "username": "...", "password": "..." }` and return an opaque token, role, expiry, and password-free user DTO.
- `/api/iam/logout`, `/api/iam/me`, `/api/iam/password`, `/api/iam/sessions`, and `/api/iam/sessions/revoke-others` provide current-user session and profile operations. Revoking other sessions preserves the presented session.
- `GET /api/iam/session` answers the session's token, role, and user to a request that carries the session (cookie or Bearer), `Cache-Control: no-store`: what a console tab that starts with the cookie but without the token calls to enter again.
- `/api/iam/users` and `/api/iam/audit` are administrator-only. Resetting a password revokes every target session.
- `GET /internal/authorize` is intended for a trusted reverse proxy. It requires the original method and URI headers. Under `/api/` it judges by method and path (GET and HEAD may come with the cookie alone, anything else needs the Bearer), rejects malformed paths, fails closed for unknown routes, and emits the selected upstream token only in `X-IAM-Upstream` on success. Any other URI is one of the console's pages: the session alone admits it, and the view is not audited.

All IAM write endpoints require a Bearer session token. The authorization endpoint accepts the session cookie for GET/HEAD requests, supporting browser terminal handshakes. Upstream tokens are never returned by login or IAM management endpoints.

## Verification limits

The package tests cover authorization, password hashing, opaque login tokens, expiry, revocation, session restore, and username uniqueness through a test store. Migration/bootstrap behavior requires PostgreSQL and must be integration-tested in an environment with PostgreSQL available.
