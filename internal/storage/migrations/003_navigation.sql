CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    cwd TEXT NOT NULL,
    data TEXT NOT NULL,
    UNIQUE(scope, cwd)
);
CREATE TABLE requirements (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id),
    data TEXT NOT NULL
);
ALTER TABLE sessions ADD COLUMN requirement_id TEXT REFERENCES requirements(id);
CREATE INDEX requirement_project ON requirements(project_id);
CREATE INDEX session_requirement ON sessions(requirement_id);
INSERT INTO schema_migrations VALUES (3, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
