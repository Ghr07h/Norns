import { ChevronDown, ChevronRight, LocateFixed } from "lucide-react";
import { useState, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import { useTranslation } from "react-i18next";
import remarkGfm from "remark-gfm";
import type { PublicEvent } from "../../api/dto";
import { formatTime, shortId } from "../../lib/format";

type AgentInfo = { role: string; name: string };

const SOURCE_LABELS: Record<string, [string, string]> = {
  runtime: ["运行时", "Runtime"],
  server: ["服务", "Server"],
  nornsd: ["服务", "Server"],
  agent: ["智能体", "Agent"],
  user: ["用户", "User"],
  norns: ["Norns", "Norns"],
};

const EVENT_LABELS: Record<string, [string, string]> = {
  "subject.created": ["创建实体", "Subject created"],
  "runtime.started": ["启动运行时", "Runtime started"],
  "runtime.cancelled": ["取消运行时", "Runtime cancelled"],
  "runtime.finished": ["完成运行时", "Runtime finished"],
  "runtime.recovery_notice": ["恢复运行时", "Runtime recovery"],
  "runtime.session_resumed": ["恢复会话", "Session resumed"],
  "instruction.dispatched": ["下发指令", "Instruction dispatched"],
  instruction: ["下发指令", "Instruction dispatched"],
  "runtime.waiting": ["等待后续指令", "Waiting for instruction"],
  "agent.text": ["输出消息", "Message"],
  "agent.tool_call": ["调用工具", "Tool call"],
  "agent.tool_result": ["返回工具结果", "Tool result"],
  "agent.result": ["完成智能体执行", "Agent completed"],
  "norns.lifecycle": ["更新 Norns 生命周期", "Norns lifecycle updated"],
  diagnostic: ["记录诊断信息", "Diagnostic recorded"],
};

function isChinese(language: string): boolean {
  return language.toLowerCase().startsWith("zh");
}

export function payloadRecord(event: PublicEvent): Record<string, unknown> | null {
  return event.payload && typeof event.payload === "object" && !Array.isArray(event.payload)
    ? event.payload as Record<string, unknown>
    : null;
}

function agentInfo(event: PublicEvent): AgentInfo | null {
  if (!event.type.startsWith("agent.")) return null;
  const value = payloadRecord(event)?.agent;
  const agent = value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : null;
  return {
    role: typeof agent?.role === "string" && agent.role ? agent.role : "main",
    name: typeof agent?.name === "string" && agent.name ? agent.name : "Coordinator",
  };
}

export function eventAgentLabel(event: PublicEvent, language: string): string | null {
  const agent = agentInfo(event);
  if (!agent) return null;
  const role = isChinese(language)
    ? agent.role === "sub" ? "子智能体" : "主智能体"
    : agent.role === "sub" ? "Sub-agent" : "Main agent";
  return `${role} · ${agent.name}`;
}

export function eventSourceLabel(source: string, language: string): string {
  const labels = SOURCE_LABELS[source];
  return labels ? labels[isChinese(language) ? 0 : 1] : source;
}

export function eventTypeLabel(type: string, language: string): string {
  const labels = EVENT_LABELS[type];
  return labels ? labels[isChinese(language) ? 0 : 1] : type;
}

function stringValue(value: unknown): string | null {
  return typeof value === "string" && value ? value : null;
}

function numberValue(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) ? value : null;
}

function displayValue(value: unknown): string {
  if (typeof value === "string") return JSON.stringify(value);
  if (value === null) return "null";
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (value === undefined) return "-";
  return JSON.stringify(value);
}

function resultContent(value: unknown): string | null {
  if (typeof value === "string") return value;
  if (!Array.isArray(value)) return null;
  const parts = value.flatMap((item) => {
    if (typeof item === "string") return [item];
    if (!item || typeof item !== "object" || Array.isArray(item)) return [];
    const record = item as Record<string, unknown>;
    return typeof record.text === "string" ? [record.text] : typeof record.content === "string" ? [record.content] : [];
  });
  return parts.length ? parts.join("\n") : null;
}

function EventMessage({ children }: { children: ReactNode }) {
  return <div className="event-message">{children}</div>;
}

function EventFact({ label, children }: { label: string; children: ReactNode }) {
  return <div className="event-fact"><span>{label}</span><div>{children}</div></div>;
}

function ToolCallContent({ payload, linkedResult, onJump }: { payload: Record<string, unknown> | null; linkedResult?: PublicEvent; onJump: (seq: string) => void }) {
  const { t } = useTranslation();
  const name = stringValue(payload?.name) ?? t("events.unknownTool");
  const input = payload?.input && typeof payload.input === "object" && !Array.isArray(payload.input)
    ? payload.input as Record<string, unknown>
    : null;
  const entries = input ? Object.entries(input) : [];
  return <div className="event-tool-call">
    <strong className="event-tool-name">{name}</strong>
    {entries.length
      ? <ul className="event-parameters">{entries.map(([key, value]) => <li key={key}><span>{key}</span><code>{displayValue(value)}</code></li>)}</ul>
      : <p className="event-empty-value">{t("events.noParameters")}</p>}
    {linkedResult && <button type="button" className="event-link" onClick={() => onJump(linkedResult.seq)}><LocateFixed />{t("events.viewResult")} #{linkedResult.seq}</button>}
  </div>;
}

function ToolResultContent({ event, payload, linkedCall, onJump }: { event: PublicEvent; payload: Record<string, unknown> | null; linkedCall?: PublicEvent; onJump: (seq: string) => void }) {
  const { t } = useTranslation();
  const agent = agentInfo(event) ?? { role: "main", name: "Coordinator" };
  const failed = payload?.is_error === true;
  const content = resultContent(payload?.content);
  return <div className="event-tool-result">
    <div className="event-result-meta">
      <EventFact label={t("events.caller")}><strong>{agent.role} · {agent.name}</strong></EventFact>
      <EventFact label={t("events.executionStatus")}><span className={`event-result-status ${failed ? "failed" : "succeeded"}`}>{failed ? t("events.failed") : t("events.succeeded")}</span></EventFact>
    </div>
    <div className="event-result-content">
      <span>{t("events.content")}</span>
      {content ? <pre>{content}</pre> : <p>{t("events.structuredContent")}</p>}
    </div>
    {linkedCall && <button type="button" className="event-link" onClick={() => onJump(linkedCall.seq)}><LocateFixed />{t("events.viewCall")} #{linkedCall.seq}</button>}
  </div>;
}

function StandardEventContent({ event, payload }: { event: PublicEvent; payload: Record<string, unknown> | null }) {
  const { t } = useTranslation();
  switch (event.type) {
    case "subject.created": {
      const ready = payload?.workspace_state === "materialized";
      return <EventMessage><p>{ready ? t("events.messages.subjectCreatedReady") : t("events.messages.subjectCreated")}</p></EventMessage>;
    }
    case "runtime.started": {
      const slot = numberValue(payload?.slot);
      const mode = stringValue(payload?.mode);
      return <EventMessage><p>{slot !== null ? t("events.messages.runtimeStartedSlot", { slot }) : mode ? t("events.messages.runtimeStartedMode", { mode }) : t("events.messages.runtimeStarted")}</p></EventMessage>;
    }
    case "instruction":
    case "instruction.dispatched": {
      const sequence = numberValue(payload?.sequence);
      const content = stringValue(payload?.content) ?? stringValue(payload?.instruction);
      return <EventMessage><p>{sequence === null ? t("events.messages.instructionDispatched") : t("events.messages.instructionDispatchedSequence", { sequence })}</p>{content && <blockquote className="event-instruction">{content}</blockquote>}</EventMessage>;
    }
    case "runtime.waiting":
      return <EventMessage><p>{t("events.messages.runtimeWaiting")}</p></EventMessage>;
    case "runtime.cancelled": {
      const reason = stringValue(payload?.reason);
      return <EventMessage><p>{payload?.queued === true ? t("events.messages.runtimeCancelledQueued") : t("events.messages.runtimeCancelled")}</p>{reason && <EventFact label={t("events.reason")}>{reason}</EventFact>}</EventMessage>;
    }
    case "runtime.finished": {
      const state = stringValue(payload?.state);
      const exitCode = numberValue(payload?.exit_code);
      return <EventMessage><p>{t("events.messages.runtimeFinished")}</p><div className="event-facts">{state && <EventFact label={t("events.state")}>{t(`status.${state}`, { defaultValue: state })}</EventFact>}{exitCode !== null && <EventFact label={t("events.exitCode")}>{exitCode}</EventFact>}{payload?.wait_error === true && <EventFact label={t("events.waitStatus")}>{t("events.waitError")}</EventFact>}</div></EventMessage>;
    }
    case "runtime.recovery_notice": {
      const previous = stringValue(payload?.previous_runtime_id);
      return <EventMessage><p>{t("events.messages.runtimeRecovery")}</p>{previous && <EventFact label={t("events.previousRuntime")}><span className="mono" title={previous}>{shortId(previous)}</span></EventFact>}</EventMessage>;
    }
    case "runtime.session_resumed": {
      const previous = stringValue(payload?.resume_session_id);
      const current = stringValue(payload?.session_id);
      return <EventMessage><p>{t("events.messages.sessionResumed")}</p><div className="event-facts">{previous && <EventFact label={t("events.previousSession")}><span className="mono" title={previous}>{shortId(previous)}</span></EventFact>}{current && <EventFact label={t("events.newSession")}><span className="mono" title={current}>{shortId(current)}</span></EventFact>}</div></EventMessage>;
    }
    case "norns.lifecycle": {
      const phase = stringValue(payload?.phase);
      const deadline = stringValue(payload?.deadline);
      return <EventMessage><p>{t("events.messages.nornsLifecycle")}</p><div className="event-facts">{phase && <EventFact label={t("events.phase")}>{phase}</EventFact>}{deadline && <EventFact label={t("events.deadline")}>{deadline}</EventFact>}</div></EventMessage>;
    }
    case "agent.result": {
      const failed = payload?.is_error === true;
      const subtype = stringValue(payload?.subtype);
      return <EventMessage><p>{failed ? t("events.messages.agentFailed") : t("events.messages.agentCompleted")}</p>{subtype && <EventFact label={t("events.resultType")}>{subtype}</EventFact>}</EventMessage>;
    }
    case "diagnostic": {
      const code = stringValue(payload?.code);
      return <EventMessage><p>{t("events.messages.diagnostic")}</p>{code && <EventFact label={t("events.diagnosticCode")}><code>{code}</code></EventFact>}</EventMessage>;
    }
    default:
      return <EventMessage><p>{t("events.messages.recorded")}</p></EventMessage>;
  }
}

export function EventRow({ event, linkedCall, linkedResult, highlighted, onJump }: { event: PublicEvent; linkedCall?: PublicEvent; linkedResult?: PublicEvent; highlighted: boolean; onJump: (seq: string) => void }) {
  const { t, i18n } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const payload = payloadRecord(event);
  const agentLabel = eventAgentLabel(event, i18n.language);
  const text = stringValue(payload?.text);
  return <article id={`event-seq-${event.seq}`} className={`event-row event-${event.type.replaceAll(".", "-")} ${highlighted ? "event-highlighted" : ""}`}>
    <header className="event-header">
      <div className="event-heading">
        <span className="event-source-badge">{eventSourceLabel(event.source, i18n.language)}</span>
        {agentLabel && <span className="event-agent">{agentLabel}</span>}
        <strong className="event-type" title={event.type}>{eventTypeLabel(event.type, i18n.language)}</strong>
      </div>
      <div className="event-meta">
        {event.runtime_id && <span className="event-runtime mono" title={event.runtime_id}>{t("events.runtimeId")} {shortId(event.runtime_id)}</span>}
        <time title={event.timestamp}>{formatTime(event.timestamp, i18n.language)}</time>
        <span className="event-seq">#{event.seq}</span>
      </div>
    </header>
    <div className="event-content">
      {event.type === "agent.text" && text
        ? <div className="event-markdown"><ReactMarkdown remarkPlugins={[remarkGfm]}>{text}</ReactMarkdown></div>
        : event.type === "agent.tool_call"
          ? <ToolCallContent payload={payload} linkedResult={linkedResult} onJump={onJump} />
          : event.type === "agent.tool_result"
            ? <ToolResultContent event={event} payload={payload} linkedCall={linkedCall} onJump={onJump} />
            : <StandardEventContent event={event} payload={payload} />}
    </div>
    <button type="button" className="event-expand" aria-expanded={expanded} onClick={() => setExpanded((value) => !value)}>{expanded ? <ChevronDown /> : <ChevronRight />}{t("events.payload")}</button>
    {expanded && <pre className="event-payload">{JSON.stringify(event.payload, null, 2)}</pre>}
  </article>;
}
