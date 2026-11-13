CREATE TABLE settings (id TEXT PRIMARY KEY, data TEXT NOT NULL);
INSERT INTO schema_migrations VALUES (2, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
