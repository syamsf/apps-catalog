-- +goose Up
-- a Tabler icon name like brand-docker, an uploaded icon wins over it
ALTER TABLE apps ADD COLUMN icon text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE apps DROP COLUMN icon;
