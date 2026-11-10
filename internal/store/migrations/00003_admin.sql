-- +goose Up

-- people show up here on their first sign-in
CREATE TABLE users (
    subject    text PRIMARY KEY,
    username   text        NOT NULL DEFAULT '',
    name       text        NOT NULL DEFAULT '',
    email      text        NOT NULL DEFAULT '',
    -- from the sso admin group at the last sign-in
    is_admin   boolean     NOT NULL DEFAULT false,
    last_login timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- sessions now point at users, sign everyone out once
DELETE FROM sessions;
ALTER TABLE sessions DROP COLUMN username, DROP COLUMN name, DROP COLUMN email, DROP COLUMN groups;
ALTER TABLE sessions ADD FOREIGN KEY (subject) REFERENCES users (subject) ON DELETE CASCADE;

CREATE TABLE groups (
    name        text PRIMARY KEY CHECK (name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    description text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE group_members (
    group_name text NOT NULL REFERENCES groups (name) ON UPDATE CASCADE ON DELETE CASCADE,
    subject    text NOT NULL REFERENCES users (subject) ON DELETE CASCADE,
    PRIMARY KEY (group_name, subject)
);
CREATE INDEX group_members_subject_idx ON group_members (subject);

ALTER TABLE apps
    ADD COLUMN runs_on    text NOT NULL DEFAULT '',
    ADD COLUMN visibility text NOT NULL DEFAULT 'everyone' CHECK (visibility IN ('everyone', 'groups')),
    ADD COLUMN icon_color text NOT NULL DEFAULT '',
    ADD COLUMN created_by text NOT NULL DEFAULT '',
    ADD COLUMN updated_by text NOT NULL DEFAULT '';

CREATE TABLE app_groups (
    app_id     text NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    group_name text NOT NULL REFERENCES groups (name) ON UPDATE CASCADE ON DELETE CASCADE,
    PRIMARY KEY (app_id, group_name)
);
CREATE INDEX app_groups_group_idx ON app_groups (group_name);

-- allowed_groups from the yaml catalog become portal groups
INSERT INTO groups (name)
SELECT DISTINCT g FROM apps, unnest(allowed_groups) g
WHERE g ~ '^[a-z0-9]+(-[a-z0-9]+)*$';
INSERT INTO app_groups (app_id, group_name)
SELECT DISTINCT id, g FROM apps, unnest(allowed_groups) g
WHERE g IN (SELECT name FROM groups);
UPDATE apps SET visibility = 'groups' WHERE cardinality(allowed_groups) > 0;

ALTER TABLE apps DROP COLUMN allowed_groups, DROP COLUMN source, DROP COLUMN last_seen;

CREATE TABLE app_icons (
    app_id       text PRIMARY KEY REFERENCES apps (id) ON DELETE CASCADE,
    content_type text        NOT NULL,
    data         bytea       NOT NULL,
    version      int         NOT NULL DEFAULT 1,
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE favorites (
    subject    text        NOT NULL,
    app_id     text        NOT NULL REFERENCES apps (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subject, app_id)
);

-- choices for the app form: categories, where an app runs, url environments
CREATE TABLE options (
    kind     text NOT NULL CHECK (kind IN ('category', 'runs_on', 'environment')),
    key      text NOT NULL CHECK (key ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    label    text NOT NULL,
    position int  NOT NULL,
    PRIMARY KEY (kind, key)
);

INSERT INTO options (kind, key, label, position) VALUES
    ('category', 'dev-tools', 'Dev tools', 1),
    ('category', 'monitoring', 'Monitoring', 2),
    ('category', 'data', 'Data', 3),
    ('category', 'admin', 'Admin', 4),
    ('category', 'business', 'Business apps', 5),
    ('runs_on', 'kubernetes', 'Kubernetes', 1),
    ('runs_on', 'docker', 'Docker', 2),
    ('runs_on', 'vm', 'Plain VM', 3),
    ('environment', 'prod', 'Production', 1),
    ('environment', 'staging', 'Staging', 2),
    ('environment', 'dev', 'Development', 3);

-- keep values the existing apps already use
INSERT INTO options (kind, key, label, position)
SELECT 'category', c, initcap(replace(c, '-', ' ')), 100 + row_number() OVER (ORDER BY c)
FROM (SELECT DISTINCT category AS c FROM apps WHERE category ~ '^[a-z0-9]+(-[a-z0-9]+)*$') x
ON CONFLICT DO NOTHING;
INSERT INTO options (kind, key, label, position)
SELECT 'environment', e, initcap(replace(e, '-', ' ')), 100 + row_number() OVER (ORDER BY e)
FROM (SELECT DISTINCT jsonb_object_keys(urls) AS e FROM apps) x
WHERE e ~ '^[a-z0-9]+(-[a-z0-9]+)*$'
ON CONFLICT DO NOTHING;

CREATE TABLE settings (
    id                 boolean PRIMARY KEY DEFAULT true CHECK (id),
    name               text NOT NULL DEFAULT 'Dev Portal',
    admin_email        text NOT NULL DEFAULT '',
    icon_color_mode    text NOT NULL DEFAULT 'auto' CHECK (icon_color_mode IN ('auto', 'accent')),
    new_app_visibility text NOT NULL DEFAULT 'groups' CHECK (new_app_visibility IN ('everyone', 'groups'))
);
INSERT INTO settings DEFAULT VALUES;

-- what admins changed, shown on the dashboard
CREATE TABLE changes (
    id            bigserial PRIMARY KEY,
    at            timestamptz NOT NULL DEFAULT now(),
    actor_subject text        NOT NULL DEFAULT '',
    actor         text        NOT NULL,
    summary       text        NOT NULL
);
CREATE INDEX changes_at_idx ON changes (at DESC);

-- +goose Down
DROP TABLE changes;
DROP TABLE settings;
DROP TABLE options;
DROP TABLE favorites;
DROP TABLE app_icons;

ALTER TABLE apps
    ADD COLUMN allowed_groups text[]      NOT NULL DEFAULT '{}',
    ADD COLUMN source         text        NOT NULL DEFAULT 'git',
    ADD COLUMN last_seen      timestamptz NOT NULL DEFAULT now();
UPDATE apps a SET allowed_groups = coalesce(
    (SELECT array_agg(group_name ORDER BY group_name) FROM app_groups WHERE app_id = a.id), '{}');
CREATE INDEX apps_source_idx ON apps (source);
DROP TABLE app_groups;
ALTER TABLE apps
    DROP COLUMN runs_on, DROP COLUMN visibility, DROP COLUMN icon_color,
    DROP COLUMN created_by, DROP COLUMN updated_by;

DROP TABLE group_members;
DROP TABLE groups;

DELETE FROM sessions;
ALTER TABLE sessions DROP CONSTRAINT sessions_subject_fkey;
ALTER TABLE sessions
    ADD COLUMN username text   NOT NULL DEFAULT '',
    ADD COLUMN name     text   NOT NULL DEFAULT '',
    ADD COLUMN email    text   NOT NULL DEFAULT '',
    ADD COLUMN groups   text[] NOT NULL DEFAULT '{}';
DROP TABLE users;
