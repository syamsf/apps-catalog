-- +goose Up
-- the owner becomes a portal group, owners that match no group are cleared
ALTER TABLE apps ALTER COLUMN owner DROP NOT NULL, ALTER COLUMN owner DROP DEFAULT;
-- record cleared owners so admins can reassign them
INSERT INTO changes (actor_subject, actor, summary)
SELECT '', 'migration', format('cleared the owner "%s" of %s, no team with that name', trim(a.owner), a.name)
FROM apps a
WHERE trim(a.owner) <> ''
  AND NOT EXISTS (SELECT 1 FROM groups g WHERE g.name = lower(trim(a.owner)))
ORDER BY a.name;
UPDATE apps a SET owner = (SELECT g.name FROM groups g WHERE g.name = lower(trim(a.owner)));
ALTER TABLE apps ADD CONSTRAINT apps_owner_fkey
    FOREIGN KEY (owner) REFERENCES groups (name) ON UPDATE CASCADE ON DELETE SET NULL;

-- +goose Down
ALTER TABLE apps DROP CONSTRAINT apps_owner_fkey;
UPDATE apps SET owner = '' WHERE owner IS NULL;
ALTER TABLE apps ALTER COLUMN owner SET DEFAULT '', ALTER COLUMN owner SET NOT NULL;
