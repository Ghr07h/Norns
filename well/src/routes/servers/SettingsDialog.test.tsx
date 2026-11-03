import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { getVersion } from "@tauri-apps/api/app";
import { beforeAll, expect, it, vi } from "vitest";
import i18n from "../../i18n";
import { wellVersion } from "../../lib/versions";
import { cacheSummary } from "../../storage/event-db";
import { SettingsDialog } from "./SettingsDialog";

vi.mock("@tauri-apps/api/app", () => ({ getVersion: vi.fn() }));
vi.mock("../../storage/event-db", () => ({
  cacheSummary: vi.fn().mockResolvedValue({ events: 0, bytes: 0 }),
  clearAllCaches: vi.fn(),
}));

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function showModal() { this.setAttribute("open", ""); };
  HTMLDialogElement.prototype.close = function close() { this.removeAttribute("open"); };
  return i18n.changeLanguage("en");
});

it("shows the manifest version when the desktop runtime is unavailable", async () => {
  vi.mocked(getVersion).mockRejectedValue(new Error("browser preview"));
  render(<QueryClientProvider client={new QueryClient()}><SettingsDialog open onClose={() => undefined} /></QueryClientProvider>);
  await waitFor(() => expect(cacheSummary).toHaveBeenCalled());
  expect(getVersion).toHaveBeenCalled();
  expect(screen.getByText(wellVersion)).toBeInTheDocument();
});

it("uses the desktop runtime version when available", async () => {
  vi.mocked(getVersion).mockResolvedValue("9.8.7");
  render(<QueryClientProvider client={new QueryClient()}><SettingsDialog open onClose={() => undefined} /></QueryClientProvider>);
  expect(await screen.findByText("9.8.7")).toBeInTheDocument();
});
