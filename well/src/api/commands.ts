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
export const createSubject = (connection: ConnectionInput, name: string | null, instruction: string | null) =>
  invokeCommand<{ subject_id: string }>("create_subject", { input: { connection, name, instruction } });
export const submitInstruction = (connection: ConnectionInput, subjectId: string, instruction: string) =>
  invokeCommand<AcceptedResult>("submit_instruction", { input: { connection, subject_id: subjectId, instruction } });
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
export const getNornsStatus = (connection: ConnectionInput, subjectId: string, taskId: string | null) =>
  invokeCommand<NornsViewResult<unknown>>("get_norns_status", { input: { connection, subject_id: subjectId, task_id: taskId } });
export const getNornsTrace = (connection: ConnectionInput, subjectId: string, explorationId: string) =>
  invokeCommand<NornsViewResult<unknown>>("get_norns_trace", { input: { connection, subject_id: subjectId, exploration_id: explorationId } });
export const getNornsVisual = (connection: ConnectionInput, subjectId: string, lang: "zh" | "en") =>
  invokeCommand<NornsViewResult<string>>("get_norns_visual", { input: { connection, subject_id: subjectId, lang } });
