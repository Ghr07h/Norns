import { ChevronDown, ChevronRight, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { CopyButton } from "./CopyButton";

function JsonNode({ name, value, depth = 0, search = "" }: { name?: string; value: unknown; depth?: number; search?: string }) {
  const [open, setOpen] = useState(depth < 2);
  const compound = value !== null && typeof value === "object";
  const text = compound ? "" : JSON.stringify(value);
  if (!compound) {
    if (search && !`${name ?? ""} ${text}`.toLowerCase().includes(search)) return null;
    return <div className="json-row" style={{ paddingLeft: depth * 16 }}><span className="json-key">{name}</span>{name !== undefined && ": "}<span className={`json-${value === null ? "null" : typeof value}`}>{text}</span><CopyButton value={text} /></div>;
  }
  const entries = Object.entries(value as Record<string, unknown>);
  const matches = !search || `${name ?? ""} ${JSON.stringify(value)}`.toLowerCase().includes(search);
  if (!matches) return null;
  return <div className="json-node"><button className="json-toggle" style={{ paddingLeft: depth * 16 }} onClick={() => setOpen((value) => !value)}>{open ? <ChevronDown /> : <ChevronRight />}<span className="json-key">{name}</span><span className="json-count">{Array.isArray(value) ? `[${entries.length}]` : `{${entries.length}}`}</span></button>{open && entries.map(([key, child]) => <JsonNode key={key} name={key} value={child} depth={depth + 1} search={search} />)}</div>;
}

export function JsonViewer({ value }: { value: unknown }) {
  const { t } = useTranslation();
  const [raw, setRaw] = useState(false);
  const [search, setSearch] = useState("");
  const text = useMemo(() => JSON.stringify(value, null, 2), [value]);
  return <div className="json-viewer"><div className="panel-toolbar"><label className="search-field"><Search /><input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={t("common.search")} /></label><div className="segmented"><button className={!raw ? "active" : ""} onClick={() => setRaw(false)}>{t("common.structured")}</button><button className={raw ? "active" : ""} onClick={() => setRaw(true)}>{t("common.raw")}</button></div><CopyButton value={text} /></div>{raw ? <pre className="json-raw">{text}</pre> : <div className="json-tree"><JsonNode value={value} search={search.trim().toLowerCase()} /></div>}</div>;
}
