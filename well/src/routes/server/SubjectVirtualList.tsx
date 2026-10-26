import { useEffect, useRef, useState } from "react";
import type { SubjectSummary } from "../../api/dto";
import { StatusBadge } from "../../components/common/StatusBadge";
import { formatTime, shortId } from "../../lib/format";
import { useTranslation } from "react-i18next";

const ROW_HEIGHT = 70;

export function SubjectVirtualList({ subjects, selected, onSelect }: { subjects: SubjectSummary[]; selected: string | null; onSelect: (id: string) => void }) {
  const { t, i18n } = useTranslation();
  const ref = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [height, setHeight] = useState(500);
  useEffect(() => { const element = ref.current; if (!element) return; const resize = new ResizeObserver(() => setHeight(element.clientHeight)); resize.observe(element); return () => resize.disconnect(); }, []);
  const start = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - 5);
  const end = Math.min(subjects.length, Math.ceil((scrollTop + height) / ROW_HEIGHT) + 5);
  return <div ref={ref} className="subject-list" onScroll={(event) => setScrollTop(event.currentTarget.scrollTop)}><div style={{ height: subjects.length * ROW_HEIGHT, position: "relative" }}>{subjects.slice(start, end).map((subject, index) => <button key={subject.id} className={`subject-row ${selected === subject.id ? "selected" : ""}`} style={{ position: "absolute", top: (start + index) * ROW_HEIGHT }} onClick={() => onSelect(subject.id)}><span className="subject-row-main"><strong>{subject.name || t("subject.unnamed")}</strong><span className="mono">{shortId(subject.id)} · {formatTime(subject.created_at, i18n.language)}</span></span><span className="subject-row-status"><StatusBadge value={subject.state} /></span></button>)}</div></div>;
}
