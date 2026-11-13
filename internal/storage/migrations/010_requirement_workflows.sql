CREATE TABLE requirement_workflows (
    id TEXT PRIMARY KEY,
    requirement_id TEXT NOT NULL UNIQUE REFERENCES requirements(id) ON DELETE CASCADE,
    data TEXT NOT NULL
);
INSERT INTO schema_migrations VALUES (10, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
