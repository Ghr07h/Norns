CREATE TABLE metadata (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    root_id TEXT NOT NULL,
    cursor_hmac_key BLOB NOT NULL CHECK (length(cursor_hmac_key) = 32),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE subjects (
    id TEXT PRIMARY KEY,
    name TEXT,
    state TEXT NOT NULL CHECK (state IN ('idle','queuing','using')),
    directory_name TEXT NOT NULL UNIQUE,
    workspace_state TEXT NOT NULL CHECK (workspace_state IN ('planned','materializing','materialized','unavailable')),
    workspace_error TEXT,
    profile_json TEXT NOT NULL CHECK (json_valid(profile_json)),
    session_id TEXT NOT NULL,
    session_initialized INTEGER NOT NULL DEFAULT 0 CHECK (session_initialized IN (0,1)),
    current_runtime_id TEXT,
    last_event_seq INTEGER NOT NULL DEFAULT 0 CHECK (last_event_seq >= 0),
    last_raw_seq INTEGER NOT NULL DEFAULT 0 CHECK (last_raw_seq >= 0),
    last_projected_raw_seq INTEGER NOT NULL DEFAULT 0 CHECK (last_projected_raw_seq >= 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (id, current_runtime_id),
    FOREIGN KEY (id, current_runtime_id) REFERENCES runtimes(subject_id, id) DEFERRABLE INITIALLY DEFERRED,
    CHECK ((state = 'idle' AND current_runtime_id IS NULL) OR (state <> 'idle' AND current_runtime_id IS NOT NULL))
);

CREATE TABLE runtimes (
    id TEXT PRIMARY KEY,
    subject_id TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('queued','running','waiting','ended','error','cancelled')),
    queue_sequence INTEGER,
    settings_slot INTEGER CHECK (settings_slot IS NULL OR settings_slot > 0),
    config_hash TEXT,
    session_mode TEXT NOT NULL CHECK (session_mode IN ('new','resume')),
    inflight_turns INTEGER NOT NULL DEFAULT 0 CHECK (inflight_turns >= 0),
    closing INTEGER NOT NULL DEFAULT 0 CHECK (closing IN (0,1)),
    close_deadline TEXT,
    queued_at TEXT,
    started_at TEXT,
    ended_at TEXT,
    state_changed_at TEXT NOT NULL,
    end_reason TEXT,
    error_code TEXT,
    error_json TEXT CHECK (error_json IS NULL OR json_valid(error_json)),
    process_json TEXT CHECK (process_json IS NULL OR json_valid(process_json)),
    usage_json TEXT CHECK (usage_json IS NULL OR json_valid(usage_json)),
    resource_json TEXT CHECK (resource_json IS NULL OR json_valid(resource_json)),
    transcript_json TEXT CHECK (transcript_json IS NULL OR json_valid(transcript_json)),
    created_at TEXT NOT NULL,
    FOREIGN KEY (subject_id) REFERENCES subjects(id) DEFERRABLE INITIALLY DEFERRED,
    UNIQUE (subject_id, id),
    CHECK (id <> subject_id),
    CHECK ((closing = 0 AND close_deadline IS NULL) OR (closing = 1 AND close_deadline IS NOT NULL)),
    CHECK ((state IN ('ended','error','cancelled') AND ended_at IS NOT NULL) OR (state IN ('queued','running','waiting') AND ended_at IS NULL))
);

CREATE TRIGGER runtimes_subject_immutable
BEFORE UPDATE OF subject_id ON runtimes
WHEN NEW.subject_id <> OLD.subject_id
BEGIN
    SELECT RAISE(ABORT, 'runtime subject_id is immutable');
END;

CREATE TABLE runtime_queue (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    runtime_id TEXT NOT NULL UNIQUE,
    subject_id TEXT NOT NULL UNIQUE,
    queued_at TEXT NOT NULL,
    FOREIGN KEY (subject_id, runtime_id) REFERENCES runtimes(subject_id, id)
);

CREATE TABLE runtime_instructions (
    runtime_id TEXT NOT NULL,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    content TEXT NOT NULL,
    byte_length INTEGER NOT NULL CHECK (byte_length >= 0),
    created_at TEXT NOT NULL,
    dispatch_group INTEGER,
    dispatch_committed_at TEXT,
    write_confirmed_at TEXT,
    delivery_error_json TEXT CHECK (delivery_error_json IS NULL OR json_valid(delivery_error_json)),
    PRIMARY KEY (runtime_id, sequence),
    FOREIGN KEY (runtime_id) REFERENCES runtimes(id)
);

CREATE TABLE runtime_config_snapshots (
    runtime_id TEXT PRIMARY KEY,
    snapshot_json TEXT NOT NULL CHECK (json_valid(snapshot_json)),
    created_at TEXT NOT NULL,
    FOREIGN KEY (runtime_id) REFERENCES runtimes(id)
);

CREATE TABLE events (
    subject_id TEXT NOT NULL,
    seq INTEGER NOT NULL CHECK (seq > 0),
    runtime_id TEXT,
    type TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    source TEXT NOT NULL,
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json)),
    origin_raw_seq INTEGER,
    origin_index INTEGER,
    PRIMARY KEY (subject_id, seq),
    FOREIGN KEY (subject_id) REFERENCES subjects(id),
    FOREIGN KEY (subject_id, runtime_id) REFERENCES runtimes(subject_id, id),
    UNIQUE (subject_id, origin_raw_seq, origin_index),
    CHECK ((origin_raw_seq IS NULL AND origin_index IS NULL) OR (origin_raw_seq IS NOT NULL AND origin_index IS NOT NULL))
);

CREATE INDEX subjects_state_created ON subjects(state, created_at DESC, id ASC);
CREATE INDEX runtimes_subject_created ON runtimes(subject_id, created_at DESC, id ASC);
CREATE INDEX runtime_queue_order ON runtime_queue(sequence ASC);
CREATE INDEX runtime_instructions_pending ON runtime_instructions(runtime_id, dispatch_committed_at, sequence ASC);
CREATE INDEX events_subject_seq ON events(subject_id, seq ASC);
CREATE INDEX events_origin ON events(subject_id, origin_raw_seq, origin_index);
