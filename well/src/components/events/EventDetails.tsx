import type { ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import { useTranslation } from "react-i18next";
import remarkGfm from "remark-gfm";
import { formatTime } from "../../lib/format";

type RecordValue = Record<string, unknown>;

function record(value: unknown): RecordValue {
  return value && typeof value === "object" && !Array.isArray(value) ? value as RecordValue : {};
}

function text(value: unknown): string | null {
  return typeof value === "string" && value ? value : null;
}

function numeric(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

export function EventMessage({ children }: { children: ReactNode }) {
  return <div className="event-message">{children}</div>;
}

export function EventFact({ label, children }: { label: string; children: ReactNode }) {
  return <div className="event-fact"><span>{label}</span><div>{children}</div></div>;
}

// JSON inside a message is authored content, distinct from the event envelope.
export function EventBody({ value, plain = false }: { value: string; plain?: boolean }) {
  if (/^\s*[\[{]/.test(value)) {
    try {
      return <pre className="event-body-json">{JSON.stringify(JSON.parse(value), null, 2)}</pre>;
    } catch { /* Mixed output or malformed JSON remains intact. */ }
  }
  return plain ? <pre>{value}</pre> : <div className="event-markdown"><ReactMarkdown remarkPlugins={[remarkGfm]}>{value}</ReactMarkdown></div>;
}

export function EventState({ value }: { value: unknown }) {
  const { t } = useTranslation();
  return <>{value === null ? t("events.details.stateCleared") : typeof value === "string" && value
    ? t(`events.details.states.${value}`, { defaultValue: value }) : t("events.details.unknown")}</>;
}

function Fact({ label, value }: { label: string; value: unknown }) {
  const { t } = useTranslation();
  if (value === undefined || value === null || value === "") return null;
  const display = typeof value === "boolean" ? t(value ? "common.yes" : "common.no")
    : typeof value === "string" || typeof value === "number" ? String(value) : null;
  return display === null ? null : <EventFact label={t(`events.details.${label}`)}><span className="breakable">{display}</span></EventFact>;
}

function Reason({ label, value }: { label: string; value: unknown }) {
  const { t } = useTranslation();
  return typeof value === "string" && value
    ? <EventFact label={t(`events.details.${label}`)}>{t(`events.details.reasons.${value}`, { defaultValue: value })}</EventFact> : null;
}

// Errors may be strings, arrays, or structured objects. Preserve their fields
// without presenting the whole event as a JSON document.
function ErrorValue({ value }: { value: unknown }) {
  const { t } = useTranslation();
  if (value === null || value === undefined) return null;
  if (typeof value === "string") return <pre>{t(`events.details.reasons.${value}`, { defaultValue: value })}</pre>;
  if (Array.isArray(value)) return <ul>{value.map((item, index) => <li key={index}><ErrorValue value={item} /></li>)}</ul>;
  if (typeof value === "object") return <div className="event-facts">{Object.entries(value).map(([key, item]) => <EventFact key={key} label={key}><ErrorValue value={item} /></EventFact>)}</div>;
  return <span>{String(value)}</span>;
}

function ErrorDetails({ value }: { value: unknown }) {
  const { t } = useTranslation();
  if (value === undefined || value === null || value === "" || (Array.isArray(value) && !value.length)) return null;
  return <EventFact label={t("events.details.error")}><ErrorValue value={value} /></EventFact>;
}

function Duration({ label, value }: { label: string; value: unknown }) {
  const { t, i18n } = useTranslation();
  const ms = numeric(value);
  return ms !== null ? <EventFact label={t(`events.details.${label}`)}>{t("events.details.seconds", { value: new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 3 }).format(ms / 1000) })}</EventFact> : null;
}

function UsageFacts({ value }: { value: unknown }) {
  const usage = record(value);
  const metrics = [
    ["totalTokens", usage.total_tokens], ["toolUses", usage.tool_uses],
    ["inputTokens", usage.input_tokens ?? usage.inputTokens], ["outputTokens", usage.output_tokens ?? usage.outputTokens],
    ["cacheReadTokens", usage.cache_read_input_tokens ?? usage.cacheReadInputTokens],
  ] as const;
  return <div className="event-facts">{metrics.map(([label, value]) => numeric(value) !== null && <Fact key={label} label={label} value={value} />)}<Duration label="duration" value={usage.duration_ms} /></div>;
}

function InstructionDetails({ type, payload }: { type: string; payload: RecordValue }) {
  const { t } = useTranslation();
  const action = type.slice("instruction.".length);
  const sequences = Array.isArray(payload.instruction_sequences)
    ? payload.instruction_sequences.filter((value) => typeof value === "number" || typeof value === "string").join(", ") : null;
  return <EventMessage>
    <p>{t(`events.details.${action}`)}</p>
    <Fact label="instructionSequences" value={sequences} />
    <Fact label="inputId" value={payload.input_uuid} />
    <Fact label="resultId" value={payload.result_id} />
    {typeof payload.is_error === "boolean" && <EventFact label={t("events.executionStatus")}>{t(payload.is_error ? "events.failed" : "events.succeeded")}</EventFact>}
    <Reason label="origin" value={record(payload.origin).kind} />
  </EventMessage>;
}

function TaskDetails({ type, payload }: { type: string; payload: RecordValue }) {
  const { t, i18n } = useTranslation();
  const patch = record(payload.patch);
  const fields = { ...payload, ...patch };
  const description = text(fields.description);
  const summary = text(fields.summary);
  const endedAt = numeric(fields.end_time);
  const date = endedAt === null ? null : new Date(endedAt);
  const action = { "agent.task_started": "taskStarted", "agent.task_progress": "taskProgress", "agent.task_updated": "taskUpdated", "agent.task_notification": "taskNotification" }[type];
  return <EventMessage>
    <p>{t(`events.details.${action}`)}</p>
    <div className="event-facts">
      <Fact label="taskId" value={fields.task_id} />
      {text(fields.task_type) && <EventFact label={t("events.details.taskType")}>{t(`events.details.${fields.task_type}`, { defaultValue: String(fields.task_type) })}</EventFact>}
      <Fact label="subagentType" value={fields.subagent_type} />
      {Object.hasOwn(fields, "status") && <EventFact label={t("events.state")}><EventState value={fields.status} /></EventFact>}
      <Fact label="lastTool" value={fields.last_tool_name} />
      <Fact label="backgrounded" value={fields.is_backgrounded} />
      {date && !Number.isNaN(date.getTime()) && <Fact label="endedAt" value={formatTime(date.toISOString(), i18n.language)} />}
      <Fact label="outputFile" value={fields.output_file} />
    </div>
    {description && <EventFact label={t("events.details.description")}><EventBody value={description} /></EventFact>}
    {summary && summary !== description && <EventFact label={t("events.details.summary")}><EventBody value={summary} /></EventFact>}
    <UsageFacts value={fields.usage} />
    <ErrorDetails value={fields.error} />
  </EventMessage>;
}

function ResultDetails({ payload }: { payload: RecordValue }) {
  const { t } = useTranslation();
  const result = text(payload.result);
  const models = Object.entries(record(payload.model_usage));
  return <EventMessage>
    <p>{t(payload.is_error === true ? "events.messages.agentFailed" : "events.messages.agentCompleted")}</p>
    <div className="event-facts">
      {text(payload.subtype) && <EventFact label={t("events.resultType")}><EventState value={payload.subtype} /></EventFact>}
      <Fact label="httpStatus" value={payload.api_error_status} />
      <Fact label="turns" value={payload.num_turns} />
      <Reason label="terminalReason" value={payload.terminal_reason} />
      <Reason label="stopReason" value={payload.stop_reason} />
      <Reason label="origin" value={payload.origin_kind} />
    </div>
    {result && <EventBody value={result} />}
    <ErrorDetails value={payload.errors} />
    {Object.keys(record(payload.usage)).length > 0 && <EventFact label={t("events.details.usage")}><UsageFacts value={payload.usage} /></EventFact>}
    {models.length > 0 && <details className="event-model-usage"><summary>{t("events.details.modelUsage")}</summary>{models.map(([model, usage]) => <EventFact key={model} label={model}><UsageFacts value={usage} /></EventFact>)}</details>}
  </EventMessage>;
}

export function EventDetails({ type, payload: value }: { type: string; payload: RecordValue | null }) {
  const { t } = useTranslation();
  const payload = value ?? {};
  switch (type) {
    case "instruction.replayed": case "instruction.accepted": case "instruction.settled":
      return <InstructionDetails type={type} payload={payload} />;
    case "agent.task_started": case "agent.task_progress": case "agent.task_updated": case "agent.task_notification":
      return <TaskDetails type={type} payload={payload} />;
    case "agent.api_retry":
      return <EventMessage><p>{t("events.details.retrying")}</p>
        <Fact label="attempt" value={payload.attempt} /><Fact label="maxRetries" value={payload.max_retries} />
        <Duration label="retryDelay" value={payload.retry_delay_ms} /><Fact label="httpStatus" value={payload.error_status} />
        <ErrorDetails value={payload.error} />
      </EventMessage>;
    case "agent.status":
      return <EventMessage><p>{t("events.details.statusChanged")}</p><EventFact label={t("events.state")}><EventState value={payload.status} /></EventFact></EventMessage>;
    case "agent.text":
      return <EventMessage>{payload.error != null && <><Fact label="model" value={payload.model} /><ErrorDetails value={payload.error} /></>}{text(payload.text) && <EventBody value={String(payload.text)} />}</EventMessage>;
    case "agent.user_message":
      return <EventMessage><p>{t("events.details.userMessage")}</p>{text(payload.text) && <EventBody value={String(payload.text)} />}</EventMessage>;
    case "agent.interrupted": {
      const body = text(payload.text);
      const known = body === "[Request interrupted by user]" || body === "[Request interrupted by user for tool use]";
      return <EventMessage><p>{t(body === "[Request interrupted by user for tool use]" ? "events.details.interruptedTools" : "events.details.interrupted")}</p>{body && !known && <EventBody value={body} />}</EventMessage>;
    }
    case "agent.result":
      return <ResultDetails payload={payload} />;
    case "runtime.error":
    case "diagnostic":
      return <EventMessage><p>{t(type === "runtime.error" ? "events.details.runtimeError" : "events.messages.diagnostic")}</p>
        {text(payload.code) && <EventFact label={t(type === "runtime.error" ? "events.errorCode" : "events.diagnosticCode")}><code>{String(payload.code)}</code></EventFact>}
        {text(payload.message) && <EventFact label={t("events.details.diagnosticMessage")}><EventBody value={String(payload.message)} plain /></EventFact>}
        <ErrorDetails value={payload.error} />
        <Fact label="instructionSequences" value={Array.isArray(payload.instruction_sequences) ? payload.instruction_sequences.join(", ") : undefined} />
        <Fact label="taskId" value={payload.task_id} /><Fact label="recoveryBlocked" value={payload.recovery_blocked} /><Fact label="processRecovery" value={payload.process_recovery} />
      </EventMessage>;
    default:
      return <EventMessage><p>{t("events.messages.recorded")}</p>{text(payload.message) && <EventBody value={String(payload.message)} plain />}</EventMessage>;
  }
}
