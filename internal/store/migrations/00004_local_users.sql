-- +goose Up

-- sso users come from the oidc login, local users sign in with a password
ALTER TABLE users
    ADD COLUMN source        text NOT NULL DEFAULT 'sso' CHECK (source IN ('sso', 'local')),
    ADD COLUMN password_hash text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX users_local_username_idx ON users (lower(username)) WHERE source = 'local';
-- null until the first sign-in
ALTER TABLE users ALTER COLUMN last_login DROP NOT NULL;

-- +goose Down
DELETE FROM users WHERE source = 'local';
DROP INDEX users_local_username_idx;
UPDATE users SET last_login = created_at WHERE last_login IS NULL;
ALTER TABLE users ALTER COLUMN last_login SET NOT NULL;
ALTER TABLE users DROP COLUMN password_hash, DROP COLUMN source;
