CREATE TABLE secrets (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL COLLATE NOCASE UNIQUE,
    data TEXT NOT NULL,
    ciphertext BLOB NOT NULL
);
INSERT INTO schema_migrations VALUES (6, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
