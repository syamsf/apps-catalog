-- +goose Up
ALTER TABLE apps ADD COLUMN allowed_groups text[] NOT NULL DEFAULT '{}';

CREATE TABLE app_status (
    app_id      text PRIMARY KEY REFERENCES apps (id) ON DELETE CASCADE,
    up          boolean     NOT NULL,
    http_status int         NOT NULL DEFAULT 0,
    latency_ms  int         NOT NULL,
    error       text        NOT NULL DEFAULT '',
    target      text        NOT NULL,
    checked_at  timestamptz NOT NULL,
    -- last time up changed
    since       timestamptz NOT NULL
);

CREATE TABLE sessions (
    token_hash bytea PRIMARY KEY,
    subject    text        NOT NULL,
    username   text        NOT NULL DEFAULT '',
    name       text        NOT NULL DEFAULT '',
    email      text        NOT NULL DEFAULT '',
    groups     text[]      NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

-- pending oidc logins, deleted on callback or after 10 minutes
CREATE TABLE oidc_logins (
    state      text PRIMARY KEY,
    nonce      text        NOT NULL,
    verifier   text        NOT NULL,
    redirect   text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE oidc_logins;
DROP TABLE sessions;
DROP TABLE app_status;
ALTER TABLE apps DROP COLUMN allowed_groups;
