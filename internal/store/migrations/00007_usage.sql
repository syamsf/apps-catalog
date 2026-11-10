-- +goose Up
CREATE TABLE searches (
    id      bigserial PRIMARY KEY,
    subject text NOT NULL,
    q       text NOT NULL,
    found   bool NOT NULL,
    at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX searches_at_idx ON searches (at);

CREATE TABLE search_dismissals (
    q  text PRIMARY KEY,
    at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE opens (
    id      bigserial PRIMARY KEY,
    subject text NOT NULL,
    app_id  text NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX opens_subject_at_idx ON opens (subject, at DESC);
CREATE INDEX opens_app_at_idx ON opens (app_id, at);

-- +goose Down
DROP TABLE opens;
DROP TABLE search_dismissals;
DROP TABLE searches;
