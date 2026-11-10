-- +goose Up
CREATE TABLE apps (
    id          text PRIMARY KEY CHECK (id ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    name        text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    owner       text        NOT NULL DEFAULT '',
    contact     text        NOT NULL DEFAULT '',
    category    text        NOT NULL DEFAULT '',
    access      text        NOT NULL DEFAULT '',
    health_url  text        NOT NULL DEFAULT '',
    urls        jsonb       NOT NULL DEFAULT '{}',
    links       jsonb       NOT NULL DEFAULT '{}',
    -- git, kubernetes, docker
    source      text        NOT NULL,
    last_seen   timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX apps_source_idx   ON apps (source);
CREATE INDEX apps_category_idx ON apps (category);
CREATE INDEX apps_owner_idx    ON apps (owner);

-- +goose Down
DROP TABLE apps;
