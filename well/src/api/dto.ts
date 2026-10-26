export type SubjectState = "idle" | "queuing" | "using";
export type RuntimeState = "queued" | "running" | "waiting" | "ended" | "error" | "cancelled";

export interface RuntimeProfile {
  agent: string;
  agent_version: string;
  capabilities: string[];
}

export interface ServiceInfo {
  service: string;
  server_id?: string;
  version: string;
  api_version: string;
  runtime_profile: RuntimeProfile;
  protocol_versions: { norns: number | null; agent_config: number };
  capacity: { concurrency: number; using: number; queuing: number };
  capabilities: string[];
}

export interface ProbeSnapshot {
  service: ServiceInfo;
  health: { status: string };
}

export interface Instruction {
  sequence: number;
  content: string;
  created_at: string;
}

export interface Runtime {
  id: string;
  subject_id?: string;
  state: RuntimeState;
  queue_position?: number | null;
  instructions: Instruction[];
  created_at: string;
  queued_at?: string | null;
  started_at: string | null;
  ended_at?: string | null;
  resources: unknown;
  closing: boolean;
  close_deadline: string | null;
  state_changed_at: string;
}

export interface SubjectSummary {
  id: string;
  name: string | null;
  state: SubjectState;
  created_at: string;
}

export interface SubjectDetail {
  id: string;
  name: string | null;
  state: SubjectState;
  runtime_profile: RuntimeProfile;
  workspace: {
    materialized: boolean;
    directory_name: string;
    path?: string | null;
  };
  last_event_seq: number;
  created_at: string;
  updated_at: string;
}

export interface SubjectPage {
  subjects: SubjectSummary[];
  next_cursor: string | null;
}

export interface QueueEntry {
  position: number;
  runtime_id: string;
  subject_id: string;
  subject_name: string | null;
  instruction_count: number;
  queued_at: string;
}

export interface QueuePage {
  runtimes: QueueEntry[];
  next_cursor: string | null;
}

export interface PublicEvent {
  seq: string;
  type: string;
  timestamp: string;
  runtime_id: string | null;
  source: string;
  payload: Record<string, unknown> | unknown[] | string | number | boolean | null;
}

export interface EventPage {
  events: PublicEvent[];
  cursor: string | null;
  next_cursor: string | null;
  has_more: boolean;
}

export interface RuntimeSummary {
  id: string;
  state: RuntimeState;
  current: boolean;
  created_at: string;
  started_at: string | null;
  ended_at: string | null;
}

export interface RuntimePage {
  runtimes: RuntimeSummary[];
  next_cursor: string | null;
  total_count: number;
}

export interface RuntimeDetail {
  id: string;
  subject_id: string;
  state: RuntimeState;
  agent: string;
  session_id: string;
  queued_at: string | null;
  started_at: string | null;
  ended_at: string | null;
  end_reason: string | null;
  settings_slot: number | null;
  config_hash: string | null;
  process: unknown;
  usage: unknown;
  resources: unknown;
  error: unknown;
  artifacts?: {
    raw_log_path?: string | null;
    readable_log_path?: string | null;
    memory_samples_path?: string | null;
    transcript_path?: string | null;
    transcript_status?: "pending" | "available" | "missing" | "failed";
  } | null;
}

export interface ClientError {
  kind: "configuration" | "connection" | "timeout" | "authentication" | "api" | "protocol" | "response_too_large";
  httpStatus: number | null;
  code: string | null;
  message: string;
  requestId: string | null;
  details: Record<string, unknown>;
}

export interface ConnectionInput {
  base_url: string;
  password: string;
}

export interface AcceptedResult {
  accepted: boolean;
  runtime?: { id: string; state: RuntimeState; queue_position?: number | null; instruction_count: number };
  subject_id?: string;
  runtime_id?: string;
  runtime_state?: RuntimeState;
}
