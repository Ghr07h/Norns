PRAGMA foreign_keys = ON;

CREATE TABLE workspace_meta (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    protocol_version INTEGER NOT NULL,
    runtime_occupied INTEGER NOT NULL CHECK (runtime_occupied IN (0, 1)),
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
    task_id TEXT,
    origin TEXT NOT NULL CHECK (origin IN ('task', 'user', 'exploration')),
    origin_cognition_id TEXT,
    origin_exploration_id TEXT,
    task_purpose_relevant INTEGER NOT NULL DEFAULT 0 CHECK (task_purpose_relevant IN (0, 1)),
    created_at TEXT NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (origin_cognition_id) REFERENCES cognitions(id),
    FOREIGN KEY (origin_exploration_id) REFERENCES explorations(id),
    CHECK ((origin = 'task' AND task_id IS NOT NULL AND origin_exploration_id IS NULL AND task_purpose_relevant = 0) OR
           (origin = 'user' AND ((task_id IS NULL AND origin_cognition_id IS NOT NULL AND origin_exploration_id IS NULL AND task_purpose_relevant = 0) OR
                                 (task_id IS NOT NULL AND origin_cognition_id IS NULL AND origin_exploration_id IS NOT NULL))) OR
           (origin = 'exploration' AND task_id IS NOT NULL AND origin_cognition_id IS NULL AND origin_exploration_id IS NOT NULL))
);

CREATE TABLE explorations (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    content TEXT NOT NULL CHECK (length(trim(content)) > 0),
    task_id TEXT NOT NULL,
    retry_of TEXT,
    status TEXT NOT NULL CHECK (status IN ('todo', 'running', 'finished')),
    executor_id TEXT,
    output_ref TEXT,
    assigned_at TEXT,
    outcome TEXT CHECK (outcome IS NULL OR outcome IN ('succeeded', 'inconclusive', 'failed', 'cancelled')),
    outcome_reason TEXT,
    created_at TEXT NOT NULL,
    finished_at TEXT,
    FOREIGN KEY (task_id) REFERENCES tasks(id),
    FOREIGN KEY (retry_of) REFERENCES explorations(id),
    CHECK (retry_of IS NULL OR retry_of <> id),
    CHECK ((status = 'todo' AND executor_id IS NULL AND output_ref IS NULL AND assigned_at IS NULL AND outcome IS NULL AND outcome_reason IS NULL AND finished_at IS NULL) OR
           (status = 'running' AND executor_id IS NOT NULL AND length(trim(executor_id)) > 0 AND output_ref IS NOT NULL AND length(trim(output_ref)) > 0 AND assigned_at IS NOT NULL AND outcome IS NULL AND outcome_reason IS NULL AND finished_at IS NULL) OR
           (status = 'finished' AND outcome IS NOT NULL AND outcome_reason IS NOT NULL AND length(trim(outcome_reason)) > 0 AND finished_at IS NOT NULL AND
             ((executor_id IS NULL AND output_ref IS NULL AND assigned_at IS NULL AND outcome = 'cancelled') OR
              (executor_id IS NOT NULL AND length(trim(executor_id)) > 0 AND output_ref IS NOT NULL AND length(trim(output_ref)) > 0 AND assigned_at IS NOT NULL))))
);

CREATE TABLE exploration_sources (
    exploration_id TEXT NOT NULL,
    cognition_id TEXT NOT NULL,
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (exploration_id, cognition_id),
    UNIQUE (exploration_id, ordinal),
    FOREIGN KEY (exploration_id) REFERENCES explorations(id),
    FOREIGN KEY (cognition_id) REFERENCES cognitions(id)
);

CREATE INDEX tasks_created ON tasks(created_at, id);
CREATE INDEX tasks_closed ON tasks(closed_at DESC, id ASC) WHERE status = 'closed';
CREATE INDEX cognitions_created ON cognitions(created_at, id);
CREATE INDEX cognitions_task_relevant ON cognitions(task_id, task_purpose_relevant, created_at, id);
CREATE INDEX cognitions_origin_cognition ON cognitions(origin_cognition_id);
CREATE INDEX cognitions_origin_exploration ON cognitions(origin_exploration_id);
CREATE INDEX explorations_task_status ON explorations(task_id, status, created_at, id);
CREATE INDEX explorations_created ON explorations(created_at, id);
CREATE INDEX explorations_retry_of ON explorations(retry_of);
CREATE INDEX exploration_sources_cognition ON exploration_sources(cognition_id);
