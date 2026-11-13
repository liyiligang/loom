CREATE TABLE requirement_workspace_branches (
    common_dir TEXT NOT NULL,
    branch TEXT NOT NULL,
    requirement_id TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    PRIMARY KEY (common_dir, branch)
);
INSERT INTO requirement_workspace_branches
SELECT json_extract(data, '$.workspace.commonDir'), json_extract(data, '$.workspace.branch'), id
FROM requirements WHERE json_extract(data, '$.workspace.branch') IS NOT NULL;
INSERT INTO schema_migrations VALUES (5, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
