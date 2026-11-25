# Astribot IAM Backend

`astribot-iam` is a Phase 1 backend service. It stores human users, opaque sessions, and audit events in PostgreSQL. It does not provision gateway service accounts or serve the IAM web UI.

## Runtime

Run `cmd/astribot-iam` with `--db-config` pointing at the existing RLark PostgreSQL configuration. Startup applies the versioned `astribot_iam_*` migrations under an advisory transaction lock and idempotently bootstraps `admin` / `astribot` (`admin`, `u_admin`) and `user` / `123` (`operator`, `u_user`). Bootstrap passwords are bcrypt-hashed at cost 12. Change them after first login. User-created and reset passwords require at least eight characters and fit bcrypt's 72-byte input limit.

The default listener is `:8081`. `--session-ttl` defaults to `168h`. Login issues a 32-byte opaque token, stores only its SHA-256 digest, and sets the HttpOnly `astribot_session` cookie with SameSite=Lax. Configure `--cookie-secure=true` when TLS terminates before the service.

`--upstream-tokens` reads a JSON object with `admin`, `operator`, and `user` string values. The file must be mode 0600 or stricter. A missing file is supported for shadow production: login and IAM management continue to work, while `/internal/authorize` returns 503. The service never reads `rlark-ui-auth`.

## HTTP API

- `POST /api/iam/login` and `POST /api/v1/auth/login` accept `{ "username": "...", "password": "..." }` and return an opaque token, role, expiry, and password-free user DTO.
- `/api/iam/logout`, `/api/iam/me`, `/api/iam/password`, `/api/iam/sessions`, and `/api/iam/sessions/revoke-others` provide current-user session and profile operations. Revoking other sessions preserves the presented session.
- `/api/iam/users` and `/api/iam/audit` are administrator-only. Resetting a password revokes every target session.
- `GET /internal/authorize` is intended for a trusted reverse proxy. It requires original method and normalized URI headers, rejects malformed paths, fails closed for unknown routes, and emits the selected upstream token only in `X-IAM-Upstream` on success.

All IAM write endpoints require a Bearer session token. The authorization endpoint accepts the session cookie for GET/HEAD requests, supporting browser terminal handshakes. Upstream tokens are never returned by login or IAM management endpoints.

## Verification limits

The package tests cover authorization, password hashing, opaque login tokens, expiry, revocation, and username uniqueness through a test store. Migration/bootstrap behavior requires PostgreSQL and must be integration-tested in an environment with PostgreSQL available.
