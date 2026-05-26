CREATE TABLE IF NOT EXISTS stop_words (
    word TEXT PRIMARY KEY NOT NULL
);

CREATE TABLE IF NOT EXISTS stop_list_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    version BIGINT NOT NULL DEFAULT 0,
    updated_at_ms BIGINT NOT NULL DEFAULT 0
);

INSERT INTO stop_list_state(id, version, updated_at_ms)
VALUES (1, 0, 0)
ON CONFLICT (id) DO NOTHING;
