CREATE TABLE astribot_iam_users (
  id TEXT PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
  disabled BOOLEAN NOT NULL DEFAULT FALSE,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_login_at TIMESTAMPTZ
);
CREATE TABLE astribot_iam_sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES astribot_iam_users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL,
  user_agent TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL DEFAULT '',
  revoked_at TIMESTAMPTZ
);
CREATE INDEX astribot_iam_sessions_user_id ON astribot_iam_sessions(user_id);
CREATE INDEX astribot_iam_sessions_expires_at ON astribot_iam_sessions(expires_at);
CREATE TABLE astribot_iam_audit (
  id BIGSERIAL PRIMARY KEY,
  ts TIMESTAMPTZ NOT NULL DEFAULT now(),
  actor TEXT NOT NULL,
  actor_id TEXT,
  action TEXT NOT NULL,
  target TEXT NOT NULL DEFAULT '',
  result TEXT NOT NULL,
  ip TEXT NOT NULL DEFAULT '',
  detail JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX astribot_iam_audit_ts ON astribot_iam_audit(ts DESC);
CREATE INDEX astribot_iam_audit_actor ON astribot_iam_audit(actor, ts DESC);
CREATE INDEX astribot_iam_audit_action ON astribot_iam_audit(action, ts DESC);
