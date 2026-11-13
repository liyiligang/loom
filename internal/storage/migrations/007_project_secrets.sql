CREATE TABLE project_secrets (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    secret_id TEXT NOT NULL REFERENCES secrets(id) ON DELETE CASCADE,
    PRIMARY KEY (project_id, secret_id)
);
INSERT INTO schema_migrations(version, applied_at) VALUES (7, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
