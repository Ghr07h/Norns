import { describe, expect, it } from "vitest";
import tokens from "./tokens.css?raw";
import global from "./global.css?raw";
import workspace from "./workspace.css?raw";
import subjectList from "../routes/server/SubjectVirtualList.tsx?raw";

function rule(source: string, selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = source.match(new RegExp(`${escaped}\\s*\\{([^}]+)\\}`));
  expect(match, `Missing CSS selector ${selector}`).not.toBeNull();
  return match![1];
}

describe("Visual-aligned interface theme", () => {
  it("keeps the core palette aligned with Norns Visual", () => {
    for (const [name, value] of Object.entries({
      bg: "#f3f5f8", ink: "#222a38", heading: "#24344f", muted: "#667387",
      line: "#e0e5ed", "line-strong": "#bdc8d8", surface: "#ffffff",
      "surface-soft": "#f7f9fc", accent: "#3563c5", "accent-soft": "#eaf0fd",
      green: "#167354", amber: "#ad650c", red: "#b83b3b",
    })) {
      expect(tokens).toContain(`--${name}: ${value};`);
    }
    expect(tokens).toContain("font-size: 15px");
    expect(rule(workspace, ".event-markdown")).toContain("font-size: 15px");
    expect(rule(workspace, ".value-row dt, .artifact-row dt")).toContain("font-size: 12px");
  });

  it("preserves the virtual list's measured row height", () => {
    const height = subjectList.match(/const ROW_HEIGHT = (\d+);/)![1];
    expect(rule(workspace, ".subject-row")).toContain(`height: ${height}px`);
  });

  it("bounds long content and keeps dialog controls outside its scroll area", () => {
    expect(rule(global, ".dialog[open]")).toContain("grid-template-rows: auto minmax(0, 1fr) auto");
    expect(rule(global, ".dialog-body")).toContain("min-height: 0");
    expect(rule(global, ".dialog-body")).toContain("overflow: auto");
    expect(rule(workspace, ".subject-panel")).toContain("overflow: auto");
    expect(rule(workspace, ".event-payload")).toContain("max-height: 420px");
  });

  it("respects reduced motion without animating layout properties", () => {
    expect(global).toContain("@media (prefers-reduced-motion: reduce)");
    expect(global).toContain("animation: none !important");
    expect(global).toContain("transition-duration: 0s !important");
    expect(global).not.toMatch(/transition:\s*all\b/);
    expect(workspace).not.toMatch(/transition:\s*all\b/);
  });
});
