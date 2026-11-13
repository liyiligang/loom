CREATE TABLE requirement_materials (
    requirement_id TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    scope TEXT NOT NULL,
    id TEXT NOT NULL,
    path TEXT NOT NULL,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    directory TEXT NOT NULL,
    branch TEXT NOT NULL,
    commit_id TEXT NOT NULL,
    hash TEXT NOT NULL,
    size INTEGER NOT NULL,
    modified_at TEXT NOT NULL,
    scan_id TEXT NOT NULL,
    PRIMARY KEY(requirement_id, scope, id),
    UNIQUE(requirement_id, scope, path)
);
CREATE INDEX material_kind ON requirement_materials(requirement_id, scope, kind);
CREATE TABLE material_links (
    requirement_id TEXT NOT NULL,
    scope TEXT NOT NULL,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    PRIMARY KEY(requirement_id, scope, source_id, target_id),
    FOREIGN KEY(requirement_id, scope, source_id) REFERENCES requirement_materials(requirement_id, scope, id) ON DELETE CASCADE,
    FOREIGN KEY(requirement_id, scope, target_id) REFERENCES requirement_materials(requirement_id, scope, id) ON DELETE CASCADE,
    CHECK(source_id <> target_id)
);
INSERT INTO schema_migrations(version, applied_at) VALUES (8, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
