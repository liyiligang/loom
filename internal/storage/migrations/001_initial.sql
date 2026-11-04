CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
CREATE TABLE endpoints (id TEXT PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    endpoint_id TEXT NOT NULL REFERENCES endpoints(id),
    provider_id TEXT NOT NULL,
    data TEXT NOT NULL,
    UNIQUE(endpoint_id, provider_id)
);
CREATE TABLE turns (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    request_id TEXT NOT NULL,
    data TEXT NOT NULL
);
CREATE UNIQUE INDEX turn_request ON turns(session_id, request_id) WHERE request_id <> '';
CREATE TABLE entries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    turn_id TEXT NOT NULL REFERENCES turns(id),
    data TEXT NOT NULL
);
CREATE TABLE interactions (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    turn_id TEXT NOT NULL REFERENCES turns(id),
    data TEXT NOT NULL
);
CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    resource TEXT NOT NULL,
    created_at TEXT NOT NULL
);
INSERT INTO schema_migrations VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
