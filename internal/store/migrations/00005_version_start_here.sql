-- +goose Up
ALTER TABLE apps
    ADD COLUMN version    text    NOT NULL DEFAULT '',
    ADD COLUMN start_here boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE apps DROP COLUMN start_here, DROP COLUMN version;
