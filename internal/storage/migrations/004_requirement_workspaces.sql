CREATE UNIQUE INDEX requirement_workspace_branch
ON requirements(json_extract(data, '$.workspace.commonDir'), json_extract(data, '$.workspace.branch'))
WHERE json_extract(data, '$.workspace.branch') IS NOT NULL;
INSERT INTO schema_migrations VALUES (4, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
