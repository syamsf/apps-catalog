-- +goose Up
CREATE TABLE access_requests (
    id         bigserial PRIMARY KEY,
    subject    text NOT NULL REFERENCES users (subject) ON DELETE CASCADE,
    app_id     text NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    note       text NOT NULL DEFAULT '',
    status     text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'declined')),
    group_name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    decided_by text NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX access_requests_pending ON access_requests (subject, app_id) WHERE status = 'pending';

-- +goose Down
DROP TABLE access_requests;
