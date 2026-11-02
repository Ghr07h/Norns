import { fireEvent, render, screen } from "@testing-library/react";
import { beforeAll, beforeEach, expect, it, vi } from "vitest";
import i18n from "../../i18n";
import type { ServerRecord } from "../../storage/registry-schema";
import { ServerDialog } from "./ServerDialog";

const server: ServerRecord = {
  id: "server",
  name: "Local server",
  base_url: "http://127.0.0.1:28081",
  remember_password: true,
  password: "saved-secret-token",
  created_at: "2026-11-02T00:00:00Z",
  updated_at: "2026-11-02T00:00:00Z",
  last_connected_at: null,
};

beforeAll(() => {
  HTMLDialogElement.prototype.showModal = function showModal() { this.setAttribute("open", ""); };
  HTMLDialogElement.prototype.close = function close() { this.removeAttribute("open"); };
});

beforeEach(async () => {
  await i18n.changeLanguage("zh");
});

it.each([undefined, server])("toggles token visibility without changing or saving the token (server: %j)", (record) => {
  const onSaved = vi.fn();
  const onClose = vi.fn();
  render(<ServerDialog open server={record} onSaved={onSaved} onClose={onClose} />);
  const input = screen.getByLabelText("Token");
  expect(input).toHaveAttribute("type", "password");
  expect(input).toHaveValue(record?.password ?? "");
  fireEvent.change(input, { target: { value: "updated-secret-token" } });

  const showButton = screen.getByRole("button", { name: "显示 Token" });
  expect(showButton).toHaveAttribute("aria-pressed", "false");
  fireEvent.click(showButton);
  expect(input).toHaveAttribute("type", "text");
  expect(input).toHaveValue("updated-secret-token");
  const hideButton = screen.getByRole("button", { name: "隐藏 Token" });
  expect(hideButton).toHaveAttribute("aria-pressed", "true");
  fireEvent.click(hideButton);
  expect(input).toHaveAttribute("type", "password");
  expect(input).toHaveValue("updated-secret-token");
  expect(onSaved).not.toHaveBeenCalled();
  expect(onClose).not.toHaveBeenCalled();
});

it("hides the token again when reopening or switching servers", () => {
  const props = { onSaved: vi.fn(), onClose: vi.fn() };
  const { rerender } = render(<ServerDialog {...props} open server={server} />);
  fireEvent.click(screen.getByRole("button", { name: "显示 Token" }));
  rerender(<ServerDialog {...props} open={false} server={server} />);
  rerender(<ServerDialog {...props} open server={server} />);
  expect(screen.getByLabelText("Token")).toHaveAttribute("type", "password");
  fireEvent.click(screen.getByRole("button", { name: "显示 Token" }));
  rerender(<ServerDialog {...props} open />);
  expect(screen.getByLabelText("Token")).toHaveAttribute("type", "password");
  expect(screen.getByLabelText("Token")).toHaveValue("");
});

it("localizes token visibility controls in English", async () => {
  await i18n.changeLanguage("en");
  render(<ServerDialog open onSaved={vi.fn()} onClose={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "Show token" }));
  expect(screen.getByRole("button", { name: "Hide token" })).toHaveAttribute("aria-pressed", "true");
});
