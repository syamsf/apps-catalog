-- +goose Up
-- for the missed searches on the dashboard
CREATE INDEX searches_missed_q_idx ON searches (q) WHERE NOT found;

-- +goose Down
DROP INDEX searches_missed_q_idx;
