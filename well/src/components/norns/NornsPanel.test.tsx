import { afterEach, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { save } from "@tauri-apps/plugin-dialog";
import { writeTextFile } from "@tauri-apps/plugin-fs";
import i18n from "../../i18n";
import { NornsPanel } from "./NornsPanel";
import { NornsViewsPanel } from "./NornsViewsPanel";
import { clearNornsViewCache } from "./view-cache";

vi.mock("@tauri-apps/plugin-dialog", () => ({ save: vi.fn() }));
vi.mock("@tauri-apps/plugin-fs", () => ({ writeTextFile: vi.fn() }));

afterEach(() => {
  clearNornsViewCache();
  vi.clearAllMocks();
  delete window.__WELL_BRIDGE__;
});

it.each(["zh", "en"] as const)("requests the %s Visual HTML and renders it only in the strict sandbox", async (language) => {
  await i18n.changeLanguage(language);
  let invocation: { command: string; args?: Record<string, unknown> } | undefined;
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    invocation = { command, args };
    return { data: "<html><body><script>document.body.dataset.ready='1'</script></body></html>", filename: "visual-20261022T080000.123456789Z-a1b2c3d4.html" } as T;
  } };
  const { container } = render(<NornsPanel serverId="server" kind="visual" enabled connection={{ base_url: "http://server", password: "secret" }} subjectId="subject" />);
  fireEvent.click(screen.getByRole("button", { name: /generate|生成/i }));
  await waitFor(() => expect(container.querySelector("iframe")).toBeInTheDocument());
  expect(invocation).toEqual({
    command: "get_norns_visual",
    args: { input: { connection: { base_url: "http://server", password: "secret" }, subject_id: "subject", lang: language } },
  });
  const frame = container.querySelector("iframe")!;
  expect(frame.getAttribute("sandbox")).toBe("allow-scripts");
  expect(frame.getAttribute("sandbox")).not.toContain("allow-same-origin");
});

it("switches the combined Norns view without exposing separate Subject tabs", async () => {
  await i18n.changeLanguage("zh");
  let command = "";
  window.__WELL_BRIDGE__ = { invoke: async <T,>(next: string) => {
    command = next;
    return { data: "<html><body>visual</body></html>", filename: "visual-20261022T080000.123456789Z-a1b2c3d4.html" } as T;
  } };
  render(<NornsViewsPanel serverId="server" enabled connection={{ base_url: "http://server", password: "secret" }} subjectId="subject" />);

  expect(screen.getByRole("button", { name: "可视化" })).toHaveClass("active");
  expect(screen.getByRole("button", { name: "全局状态" })).not.toHaveClass("active");
  fireEvent.click(screen.getByRole("button", { name: "生成" }));
  await waitFor(() => expect(command).toBe("get_norns_visual"));
});

it("restores a generated view after the panel is unmounted", async () => {
  await i18n.changeLanguage("zh");
  let calls = 0;
  window.__WELL_BRIDGE__ = { invoke: async <T,>() => {
    calls += 1;
    return { data: { marker: "cached-result" }, filename: "status-20261022T080000.123456789Z-b2c3d4e5.json" } as T;
  } };

  const first = render(<NornsPanel serverId="server" kind="status" enabled connection={{ base_url: "http://server", password: "secret" }} subjectId="subject" />);
  fireEvent.click(screen.getByRole("button", { name: "生成" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "重新生成" })).toBeInTheDocument());
  first.unmount();

  render(<NornsPanel serverId="server" kind="status" enabled connection={{ base_url: "http://server", password: "secret" }} subjectId="subject" />);
  expect(screen.getByRole("button", { name: "重新生成" })).toBeInTheDocument();
  expect(calls).toBe(1);
});

it("saves a generated view with the server file timestamp and suffix", async () => {
  await i18n.changeLanguage("zh");
  window.__WELL_BRIDGE__ = { invoke: async <T,>() => ({
    data: "<!doctype html><html><body>visual</body></html>",
    filename: "visual-20261022T080000.123456789Z-c3d4e5f6.html",
  }) as T };
  vi.mocked(save).mockResolvedValue("/tmp/well-visual.html");

  render(<NornsPanel serverId="server" kind="visual" enabled connection={{ base_url: "http://server", password: "secret" }} subjectId="subject" />);
  fireEvent.click(screen.getByRole("button", { name: "生成" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "保存视图" })).toBeInTheDocument());
  fireEvent.click(screen.getByRole("button", { name: "保存视图" }));

  await waitFor(() => expect(writeTextFile).toHaveBeenCalledWith(
    "/tmp/well-visual.html",
    "<!doctype html><html><body>visual</body></html>",
  ));
  expect(save).toHaveBeenCalledWith({
    defaultPath: "well-visual-20261022T080000.123456789Z-c3d4e5f6.html",
    filters: [{ name: "HTML", extensions: ["html"] }],
  });
  const notice = screen.getByText("视图已保存。");
  expect(notice).toBeInTheDocument();
  expect(notice.parentElement).toHaveClass("norns-view-feedback");
});

it("defaults to rendered Markdown, separates format caches and saves the original document", async () => {
  await i18n.changeLanguage("en");
  const requests: string[] = [];
  const markdown = "# Status View\n\n## Task\n\n> task_1\n\n| ID | Title |\n| --- | --- |\n| cog_1 | Finding |\n";
  window.__WELL_BRIDGE__ = { invoke: async <T,>(_command: string, args?: Record<string, unknown>) => {
    const { format } = args!.input as { format: string }; requests.push(format);
    return { data: format === "md" ? markdown : format === "yaml" ? "task: {id: task_1}\n" : '{"task":{"id":"task_1"}}', filename: `status-test.${format}` } as T;
  } };
  vi.mocked(save).mockResolvedValue("/tmp/status.md");
  render(<NornsPanel serverId="s" kind="status" connection={{ base_url: "http://server", password: "secret" }} subjectId="subject" />);
  expect(screen.getByRole("combobox", { name: "Output format" })).toHaveValue("md");
  fireEvent.click(screen.getByRole("button", { name: "Generate" }));
  expect(await screen.findByRole("heading", { name: "Status View" })).toBeInTheDocument();
  expect(screen.getByRole("table")).toHaveTextContent("Finding");
  fireEvent.click(screen.getByRole("button", { name: "Save view" }));
  await waitFor(() => expect(writeTextFile).toHaveBeenCalledWith("/tmp/status.md", markdown));
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "yaml" } });
  expect(screen.queryByRole("heading", { name: "Status View" })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Generate" }));
  await waitFor(() => expect(document.querySelector(".norns-code pre")).toHaveTextContent("task:"));
  expect(document.querySelector(".norns-code pre")!.textContent).toBe("task:\n  id: task_1\n");
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "json" } });
  fireEvent.click(screen.getByRole("button", { name: "Generate" }));
  expect(await screen.findByText('"task_1"')).toBeInTheDocument();
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "md" } });
  expect(screen.getByRole("heading", { name: "Status View" })).toBeInTheDocument();
  expect(requests).toEqual(["md", "yaml", "json"]);
});

it.each(["WORKSPACE_NOT_INITIALIZED", "NORNS_UNAVAILABLE"])("keeps the Norns entry usable and presents %s", async (code) => {
  await i18n.changeLanguage("en");
  window.__WELL_BRIDGE__ = { invoke: async () => { throw { code, message: code }; } };
  render(<NornsPanel serverId="s" kind="status" enabled={false} connection={{ base_url: "http://server", password: "secret" }} subjectId="subject" />);
  fireEvent.click(screen.getByRole("button", { name: "Generate" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(code === "NORNS_UNAVAILABLE" ? "current service" : "not a Norns workspace");
  expect(screen.getByRole("button", { name: "Generate" })).toBeEnabled();
});

it("queries both Cog and Exp in the Norns information bar", async () => {
  await i18n.changeLanguage("en");
  const calls: unknown[] = [];
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => { calls.push([command, args?.input]); return { id: "cog_1", content: "finding" } as T; } };
  render(<NornsViewsPanel serverId="s" enabled={false} connection={{ base_url: "http://server", password: "secret" }} subjectId="subject" />);
  fireEvent.click(screen.getByRole("button", { name: "Cognition" }));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "cog_1" } });
  fireEvent.click(screen.getByRole("button", { name: "Query" }));
  expect(await screen.findByText('"finding"')).toBeInTheDocument();
  expect(calls[0]).toEqual(["get_norns_entity", { connection: { base_url: "http://server", password: "secret" }, subject_id: "subject", kind: "cognition", id: "cog_1" }]);
  fireEvent.click(screen.getByRole("button", { name: "Exploration" }));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "exp_1" } });
  fireEvent.click(screen.getByRole("button", { name: "Query" }));
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]).toEqual(["get_norns_entity", expect.objectContaining({ kind: "exploration", id: "exp_1" })]);
});
