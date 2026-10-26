import type { WellBridge } from "../api/bridge";
import { REGISTRY_KEY } from "../storage/registry-schema";

export const DEMO_SERVER_ID = "11111111-1111-4111-8111-111111111111";
const REMOTE_ID = "22222222-2222-4222-8222-222222222222";
const SUBJECT_A = "33333333-3333-4333-8333-333333333333";
const SUBJECT_B = "44444444-4444-4444-8444-444444444444";
const RUNTIME_A = "55555555-5555-4555-8555-555555555555";

const runtime = {
  id: RUNTIME_A,
  subject_id: SUBJECT_A,
  state: "waiting",
  instructions: [],
  created_at: "2026-10-14T07:30:00.000Z",
  queued_at: "2026-10-14T07:30:00.000Z",
  started_at: "2026-10-14T07:30:04.000Z",
  ended_at: null,
  resources: { memory_current_bytes: 382_730_240, memory_peak_bytes: 621_805_568, memory_limit_bytes: 8_589_934_592, memory_accounting: "process_tree_rss" },
  closing: false,
  close_deadline: null,
  state_changed_at: "2026-10-14T07:34:18.000Z",
};

const subjects = [
  { id: SUBJECT_A, name: "Inspect release configuration", state: "using", created_at: "2026-10-14T07:29:55.000Z" },
  { id: SUBJECT_B, name: "Linux packaging validation", state: "queuing", created_at: "2026-10-14T07:25:00.000Z" },
  { id: "77777777-7777-4777-8777-777777777777", name: "Document protocol examples", state: "idle", created_at: "2026-10-13T11:00:00.000Z" },
];

const service = {
  service: "nornsd",
  server_id: REMOTE_ID,
  version: "0.1.0",
  api_version: "v1",
  runtime_profile: { agent: "cc", agent_version: "2.0.31", capabilities: ["norns"] },
  protocol_versions: { norns: 1, agent_config: 1 },
  capacity: { concurrency: 4, using: 1, queuing: 1 },
  capabilities: ["runtime_queue", "norns_views"],
};

function input(args?: Record<string, unknown>): Record<string, unknown> {
  return (args?.input ?? {}) as Record<string, unknown>;
}

const bridge: WellBridge = {
  async invoke<T>(command: string, args?: Record<string, unknown>): Promise<T> {
    const request = input(args);
    const subjectId = String(request.subject_id ?? SUBJECT_A);
    const result: unknown = command === "normalize_server_url" ? String(args?.input).replace(/\/$/, "")
      : command === "probe_server" ? { service, health: { status: "ok" } }
      : command === "list_subjects_page" ? { subjects, next_cursor: null }
      : command === "get_subject" ? {
        id: subjectId,
        name: subjects.find((item) => item.id === subjectId)?.name ?? "Demo subject",
        state: subjectId === SUBJECT_A ? "using" : subjectId === SUBJECT_B ? "queuing" : "idle",
        runtime_profile: service.runtime_profile,
        workspace: { materialized: true, directory_name: `20261014T073000Z_${subjectId}`, path: `/srv/nornsd/subjects/20261014T073000Z_${subjectId}` },
        last_event_seq: 6,
        created_at: "2026-10-14T07:29:55.000Z",
        updated_at: "2026-10-14T07:34:18.000Z",
      }
      : command === "get_current_runtime" ? { runtime: subjectId === SUBJECT_A ? runtime : null }
      : command === "list_queue_page" ? { runtimes: [{ position: 1, runtime_id: "66666666-6666-4666-8666-666666666666", subject_id: SUBJECT_B, subject_name: "Linux packaging validation", instruction_count: 2, queued_at: "2026-10-14T07:25:01.000Z" }], next_cursor: null }
      : command === "list_events" ? { events: [
        { seq: "1", type: "instruction", timestamp: "2026-10-14T07:30:00.000Z", runtime_id: RUNTIME_A, source: "user", payload: { instruction: "Review the release configuration and identify platform-specific risks." } },
        { seq: "2", type: "runtime.started", timestamp: "2026-10-14T07:30:04.000Z", runtime_id: RUNTIME_A, source: "nornsd", payload: { agent: "cc" } },
        { seq: "3", type: "agent.text", timestamp: "2026-10-14T07:31:12.000Z", runtime_id: RUNTIME_A, source: "agent", payload: { text: "I found two packaging assumptions that should be made platform-specific.", agent: { role: "sub", name: "Explorer-For-Packaging", source: "norns:explorer-for-packaging" } } },
        { seq: "4", type: "agent.tool_call", timestamp: "2026-10-14T07:31:19.000Z", runtime_id: RUNTIME_A, source: "agent", payload: { tool_call_id: "tool-demo-1", name: "Read", input: { file_path: "src-tauri/tauri.conf.json" }, agent: { role: "main", name: "Coordinator" } } },
        { seq: "5", type: "agent.tool_result", timestamp: "2026-10-14T07:31:20.000Z", runtime_id: RUNTIME_A, source: "agent", payload: { tool_call_id: "tool-demo-1", tool_call_seq: 4, content: "Read 914 bytes from src-tauri/tauri.conf.json", is_error: false, agent: { role: "main", name: "Coordinator" } } },
        { seq: "6", type: "runtime.waiting", timestamp: "2026-10-14T07:34:18.000Z", runtime_id: RUNTIME_A, source: "nornsd", payload: { reason: "agent_turn_completed" } },
      ], cursor: "demo-events-6", next_cursor: null, has_more: false }
      : command === "get_event_updates" ? { has_updates: false }
      : command === "list_runtimes_page" ? { runtimes: [{ id: RUNTIME_A, state: "waiting", current: true, created_at: runtime.created_at, started_at: runtime.started_at, ended_at: null }, { id: "88888888-8888-4888-8888-888888888888", state: "cancelled", current: false, created_at: "2026-10-13T08:00:00.000Z", started_at: "2026-10-13T08:00:02.000Z", ended_at: "2026-10-13T08:12:00.000Z" }], next_cursor: null, total_count: 2 }
      : command === "get_runtime" ? { id: String(request.runtime_id), subject_id: subjectId, state: String(request.runtime_id) === RUNTIME_A ? "waiting" : "cancelled", agent: "cc", session_id: String(request.runtime_id), queued_at: "2026-10-13T08:00:00.000Z", started_at: "2026-10-13T08:00:02.000Z", ended_at: String(request.runtime_id) === RUNTIME_A ? null : "2026-10-13T08:12:00.000Z", end_reason: String(request.runtime_id) === RUNTIME_A ? null : "user_graceful_stop", settings_slot: 1, config_hash: "sha256:6f334fa310df", process: String(request.runtime_id) === RUNTIME_A ? null : { exit_code: 0, signal: null }, usage: { input_tokens: 12543, output_tokens: 2190 }, resources: runtime.resources, error: null, artifacts: { raw_log_path: `/srv/nornsd/subjects/${subjectId}/.nornsd/log/raw.jsonl`, readable_log_path: `/srv/nornsd/subjects/${subjectId}/.nornsd/log/readable.jsonl`, memory_samples_path: `/srv/nornsd/subjects/${subjectId}/.nornsd/runtimes/${String(request.runtime_id)}/memory-samples.jsonl`, transcript_path: String(request.runtime_id) === RUNTIME_A ? null : `/srv/nornsd/subjects/${subjectId}/.nornsd/runtimes/${String(request.runtime_id)}/${String(request.runtime_id)}.jsonl`, transcript_status: String(request.runtime_id) === RUNTIME_A ? "pending" : "available" } }
      : command === "get_norns_status" ? { data: { task: { id: "release-check", state: "open" }, explorations: [{ id: "packaging", status: "ready" }] }, filename: "status-20261014T073500.000000000Z-a1b2c3d4.json" }
      : command === "get_norns_trace" ? { data: { exploration_id: String(request.exploration_id), nodes: [{ id: "root", state: "visited" }, { id: "platform", state: "active" }] }, filename: "trace-20261014T073500.000000000Z-b2c3d4e5.json" }
      : command === "get_norns_visual" ? { data: "<!doctype html><html><body style='margin:0;font:14px system-ui;background:#f7f9f8;color:#202526'><main style='padding:28px'><h2>Norns task graph</h2><p>release-check / packaging</p><canvas id='c' width='720' height='320'></canvas><script>const c=document.querySelector('#c'),x=c.getContext('2d');x.strokeStyle='#087b70';x.lineWidth=3;x.beginPath();x.moveTo(80,160);x.lineTo(360,80);x.lineTo(640,160);x.stroke();for(const [a,b] of [[80,160],[360,80],[640,160]]){x.fillStyle='#17211f';x.beginPath();x.arc(a,b,18,0,Math.PI*2);x.fill()}</script></main></body></html>", filename: "visual-20261014T073500.000000000Z-c3d4e5f6.html" }
      : command === "create_subject" ? { subject_id: SUBJECT_A }
      : command === "submit_instruction" || command === "stop_runtime" ? { accepted: true }
      : null;
    return result as T;
  },
};

export function installDemoBridge(): void {
  const now = new Date().toISOString();
  localStorage.setItem(REGISTRY_KEY, JSON.stringify({ version: 1, servers: [{ id: DEMO_SERVER_ID, name: "Local development", base_url: "http://127.0.0.1:28081", remote_server_id: REMOTE_ID, remember_password: true, password: "demo", created_at: now, updated_at: now, last_connected_at: now }] }));
  window.__WELL_BRIDGE__ = bridge;
}
