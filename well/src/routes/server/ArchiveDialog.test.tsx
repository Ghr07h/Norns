import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import i18n from "../../i18n";
import { ArchiveDialog } from "./ArchiveDialog";

vi.mock("@tauri-apps/api/event", () => ({ listen: vi.fn(async () => () => undefined) }));
beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute("open", ""); };
  HTMLDialogElement.prototype.close = function () { this.removeAttribute("open"); };
});
afterEach(() => { delete window.__WELL_BRIDGE__; });
const subjectId = "11111111-1111-4111-8111-111111111111";
const runtimeId = "22222222-2222-4222-8222-222222222222";
const connection = { base_url: "http://server", password: "token" };

it.each([
  ["root", "subject", undefined, undefined],
  ["rundata", "rundata", undefined, undefined],
  ["subject", "subject", subjectId, undefined],
  ["subject-rundata", "rundata", subjectId, undefined],
  ["runtime", "rundata", subjectId, runtimeId],
])("maps archive choice %s to the exact API scope", async (kind, scope, subject, runtime) => {
  await i18n.changeLanguage("en");
  const download = vi.fn();
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string, args?: Record<string, unknown>) => {
    if (command === "list_runtimes_page") return { runtimes: [], next_cursor: null } as T;
    download(command, args); return "/tmp/archive.tar.gz" as T;
  } };
  render(<ArchiveDialog open connection={connection} subjects={[{ id: subjectId, name: "Subject", state: "idle", created_at: "now" }]} selectedSubject={subjectId} onClose={vi.fn()} />);
  fireEvent.change(screen.getByRole("combobox", { name: "Archive scope" }), { target: { value: kind } });
  if (kind === "runtime") fireEvent.change(screen.getByRole("combobox", { name: "Runtime ID" }), { target: { value: runtimeId } });
  fireEvent.click(screen.getByRole("button", { name: "Download" }));
  await waitFor(() => expect(download).toHaveBeenCalledWith("download_archive", { input: { connection, request_id: expect.any(String), scope, subject_id: subject, runtime_id: runtime } }));
  expect(await screen.findByText("/tmp/archive.tar.gz")).toBeInTheDocument();
});

it("cancels a pending download without automatically retrying", async () => {
  await i18n.changeLanguage("en");
  const calls: string[] = [];
  let release: (value: unknown) => void = () => undefined;
  window.__WELL_BRIDGE__ = { invoke: async <T,>(command: string) => {
    calls.push(command);
    if (command === "download_archive") return await new Promise<unknown>((resolve) => { release = resolve; }) as T;
    release(null); return null as T;
  } };
  render(<ArchiveDialog open connection={connection} subjects={[]} selectedSubject={null} onClose={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "Download" }));
  fireEvent.click(await screen.findByRole("button", { name: "Cancel download" }));
  await waitFor(() => expect(calls).toEqual(["download_archive", "cancel_archive"]));
  expect(screen.getByRole("button", { name: "Download" })).toBeEnabled();
});
