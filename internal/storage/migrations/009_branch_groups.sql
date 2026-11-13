CREATE TABLE requirement_branch_groups (
    id TEXT PRIMARY KEY,
    requirement_id TEXT NOT NULL UNIQUE REFERENCES requirements(id) ON DELETE CASCADE,
    common_dir TEXT NOT NULL,
    prefix TEXT NOT NULL,
    UNIQUE(common_dir, prefix)
);
INSERT INTO schema_migrations VALUES (9, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
