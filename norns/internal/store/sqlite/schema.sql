PRAGMA foreign_keys = ON;

CREATE TABLE workspace_meta (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    protocol_version INTEGER NOT NULL,
    runtime_occupied INTEGER NOT NULL CHECK (runtime_occupied IN (0, 1)),
    last_cognition_number INTEGER NOT NULL DEFAULT 0 CHECK (typeof(last_cognition_number) = 'integer' AND last_cognition_number >= 0),
    last_exploration_number INTEGER NOT NULL DEFAULT 0 CHECK (typeof(last_exploration_number) = 'integer' AND last_exploration_number >= 0),
    created_at TEXT NOT NULL
);

CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    purpose TEXT NOT NULL CHECK (length(trim(purpose)) > 0),
    init_cognition_ref TEXT NOT NULL,
    max_concurrency INTEGER NOT NULL CHECK (max_concurrency > 0),
    status TEXT NOT NULL CHECK (status IN ('open', 'closed')),
    outcome TEXT CHECK (outcome IS NULL OR outcome IN ('succeeded', 'unachievable', 'failed', 'cancelled')),
    outcome_reason TEXT,
    created_at TEXT NOT NULL,
    closed_at TEXT,
    FOREIGN KEY (init_cognition_ref) REFERENCES cognitions(id) DEFERRABLE INITIALLY DEFERRED,
    CHECK ((status = 'open' AND outcome IS NULL AND outcome_reason IS NULL AND closed_at IS NULL) OR
           (status = 'closed' AND outcome IS NOT NULL AND outcome_reason IS NOT NULL AND length(trim(outcome_reason)) > 0 AND closed_at IS NOT NULL))
);
CREATE UNIQUE INDEX tasks_one_open ON tasks(status) WHERE status = 'open';

CREATE TABLE cognitions (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    content TEXT NOT NULL CHECK (length(trim(content)) > 0),
    meta TEXT NOT NULL DEFAULT '{}' CHECK (CASE WHEN json_valid(meta) THEN json_type(meta) = 'object' ELSE 0 END),
    task_id TEXT,
    origin TEXT NOT NULL CHECK (origin IN ('task', 'user', 'exploration')),
    origin_task_id TEXT,
    origin_exploration_id TEXT,
    task_purpose_relevant INTEGER NOT NULL DEFAULT 0 CHECK (task_purpose_relevant IN (0, 1)),
    created_at TEXT NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (origin_task_id) REFERENCES tasks(id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (origin_exploration_id) REFERENCES explorations(id),
    CHECK ((origin = 'task' AND task_id IS NOT NULL AND origin_task_id IS NOT NULL AND origin_task_id = task_id AND origin_exploration_id IS NULL AND task_purpose_relevant = 0) OR
           (origin = 'user' AND ((task_id IS NULL AND origin_task_id IS NOT NULL AND origin_exploration_id IS NULL AND task_purpose_relevant = 0) OR
                                 (task_id IS NOT NULL AND origin_task_id IS NULL AND origin_exploration_id IS NOT NULL))) OR
           (origin = 'exploration' AND task_id IS NOT NULL AND origin_task_id IS NULL AND origin_exploration_id IS NOT NULL))
);

CREATE TABLE explorations (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    content TEXT NOT NULL CHECK (length(trim(content)) > 0),
    task_id TEXT NOT NULL,
    sources TEXT NOT NULL CHECK (CASE WHEN json_valid(sources) THEN json_type(sources) = 'array' AND json_array_length(sources) > 0 ELSE 0 END),
    hints TEXT NOT NULL DEFAULT '[]' CHECK (CASE WHEN json_valid(hints) THEN json_type(hints) = 'array' ELSE 0 END),
    retry_of TEXT,
    status TEXT NOT NULL CHECK (status IN ('todo', 'running', 'finished')),
    explorer_id TEXT,
    output_ref TEXT,
    assigned_at TEXT,
    outcome TEXT CHECK (outcome IS NULL OR outcome IN ('succeeded', 'inconclusive', 'failed', 'cancelled')),
    outcome_reason TEXT,
    created_at TEXT NOT NULL,
    finished_at TEXT,
    FOREIGN KEY (task_id) REFERENCES tasks(id),
    FOREIGN KEY (retry_of) REFERENCES explorations(id),
    CHECK (retry_of IS NULL OR retry_of <> id),
    CHECK ((status = 'todo' AND explorer_id IS NULL AND output_ref IS NULL AND assigned_at IS NULL AND outcome IS NULL AND outcome_reason IS NULL AND finished_at IS NULL) OR
           (status = 'running' AND explorer_id IS NOT NULL AND length(trim(explorer_id)) > 0 AND output_ref IS NOT NULL AND length(trim(output_ref)) > 0 AND assigned_at IS NOT NULL AND outcome IS NULL AND outcome_reason IS NULL AND finished_at IS NULL) OR
           (status = 'finished' AND outcome IS NOT NULL AND outcome_reason IS NOT NULL AND length(trim(outcome_reason)) > 0 AND finished_at IS NOT NULL AND
             ((explorer_id IS NULL AND output_ref IS NULL AND assigned_at IS NULL AND outcome = 'cancelled') OR
              (explorer_id IS NOT NULL AND length(trim(explorer_id)) > 0 AND output_ref IS NOT NULL AND length(trim(output_ref)) > 0 AND assigned_at IS NOT NULL))))
);

CREATE INDEX tasks_created ON tasks(created_at, id);
CREATE INDEX tasks_closed ON tasks(closed_at DESC, id ASC) WHERE status = 'closed';
CREATE INDEX cognitions_created ON cognitions(created_at, id);
CREATE INDEX cognitions_task_relevant ON cognitions(task_id, task_purpose_relevant, created_at, id);
CREATE INDEX cognitions_origin_task ON cognitions(origin_task_id);
CREATE INDEX cognitions_origin_exploration ON cognitions(origin_exploration_id);
CREATE INDEX explorations_task_status ON explorations(task_id, status, created_at, id);
CREATE INDEX explorations_created ON explorations(created_at, id);
CREATE INDEX explorations_retry_of ON explorations(retry_of);
