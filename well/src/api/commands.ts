import { invokeCommand } from "./bridge";
import type {
  AcceptedResult,
  ConnectionInput,
  EventPage,
  ProbeSnapshot,
  QueuePage,
  Runtime,
  RuntimeDetail,
  RuntimePage,
  SubjectDetail,
  SubjectPage,
} from "./dto";

export interface NornsViewResult<T> {
  data: T;
  filename: string | null;
}

export const normalizeServerUrl = (input: string) => invokeCommand<string>("normalize_server_url", { input });
export const probeServer = (connection: ConnectionInput) => invokeCommand<ProbeSnapshot>("probe_server", { connection });
export const listSubjectsPage = (connection: ConnectionInput, cursor: string | null) =>
  invokeCommand<SubjectPage>("list_subjects_page", { input: { connection, cursor } });
export const getSubject = (connection: ConnectionInput, subjectId: string) =>
  invokeCommand<SubjectDetail>("get_subject", { input: { connection, subject_id: subjectId } });
export type ViewFormat = "md" | "yaml" | "json";
export type RuntimeConfig = Record<string, unknown>;

export const createSubject = (connection: ConnectionInput, name: string | null, instruction: string | null, config?: RuntimeConfig) =>
  invokeCommand<{ subject_id: string }>("create_subject", { input: { connection, name, instruction, config } });
export const submitInstruction = (connection: ConnectionInput, subjectId: string, instruction: string, config?: RuntimeConfig, resumeSession = false) =>
  invokeCommand<AcceptedResult>("submit_instruction", { input: { connection, subject_id: subjectId, instruction, config, resume_session: resumeSession } });
export const getCurrentRuntime = (connection: ConnectionInput, subjectId: string) =>
  invokeCommand<{ runtime: Runtime | null }>("get_current_runtime", { input: { connection, subject_id: subjectId } });
export const stopRuntime = (connection: ConnectionInput, subjectId: string, mode: "graceful" | "force") =>
  invokeCommand<AcceptedResult>("stop_runtime", { input: { connection, subject_id: subjectId, mode } });
export const listQueuePage = (connection: ConnectionInput, cursor: string | null) =>
  invokeCommand<QueuePage>("list_queue_page", { input: { connection, cursor } });
export const listEvents = (
  connection: ConnectionInput,
  subjectId: string,
  options: { after?: string; cursor?: string; limit?: number } = {},
) => invokeCommand<EventPage>("list_events", { input: { connection, subject_id: subjectId, ...options } });
export const getEventUpdates = (connection: ConnectionInput, subjectId: string, cursor: string) =>
  invokeCommand<{ has_updates: boolean }>("get_event_updates", { input: { connection, subject_id: subjectId, cursor } });
export const listRuntimesPage = (connection: ConnectionInput, subjectId: string, cursor: string | null) =>
  invokeCommand<RuntimePage>("list_runtimes_page", { input: { connection, subject_id: subjectId, cursor } });
export const getRuntime = (connection: ConnectionInput, subjectId: string, runtimeId: string) =>
  invokeCommand<RuntimeDetail>("get_runtime", { input: { connection, subject_id: subjectId, runtime_id: runtimeId } });
export const getNornsStatus = (connection: ConnectionInput, subjectId: string, taskId: string | null, format: ViewFormat = "md") =>
  invokeCommand<NornsViewResult<unknown>>("get_norns_status", { input: { connection, subject_id: subjectId, task_id: taskId, format } });
export const getNornsTrace = (connection: ConnectionInput, subjectId: string, explorationId: string, format: ViewFormat = "md") =>
  invokeCommand<NornsViewResult<unknown>>("get_norns_trace", { input: { connection, subject_id: subjectId, exploration_id: explorationId, format } });
export const getNornsVisual = (connection: ConnectionInput, subjectId: string, lang: "zh" | "en") =>
  invokeCommand<NornsViewResult<string>>("get_norns_visual", { input: { connection, subject_id: subjectId, lang } });

export const getNornsEntity = (connection: ConnectionInput, subjectId: string, kind: "cognition" | "exploration", id: string) =>
  invokeCommand<unknown>("get_norns_entity", { input: { connection, subject_id: subjectId, kind, id } });
export const downloadArchive = (connection: ConnectionInput, requestId: string, scope: "subject" | "rundata", subjectId?: string, runtimeId?: string) =>
  invokeCommand<string | null>("download_archive", { input: { connection, request_id: requestId, scope, subject_id: subjectId, runtime_id: runtimeId } });
export const cancelArchive = (requestId: string) => invokeCommand<void>("cancel_archive", { requestId });
