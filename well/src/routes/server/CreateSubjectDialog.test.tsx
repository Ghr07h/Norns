import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, expect, it, vi } from "vitest";
import i18n from "../../i18n";
import { CreateSubjectDialog } from "./CreateSubjectDialog";

beforeAll(async () => {
  await i18n.changeLanguage("en");
  HTMLDialogElement.prototype.showModal = function () { this.setAttribute("open", ""); };
  HTMLDialogElement.prototype.close = function () { this.removeAttribute("open"); };
});
afterEach(() => { delete window.__WELL_BRIDGE__; });

it("only submits configuration inside the requested initial Runtime", async () => {
  const invoke = vi.fn(async () => ({ subject_id: "new-subject" }));
  window.__WELL_BRIDGE__ = { invoke: invoke as never };
  const onCreated = vi.fn();
  render(<CreateSubjectDialog open connection={{ base_url: "http://server", password: "token" }} onClose={vi.fn()} onCreated={onCreated} />);
  expect(screen.queryByRole("textbox", { name: "Runtime configuration (optional JSON)" })).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.change(screen.getByRole("textbox", { name: /^Instruction/ }), { target: { value: "Investigate" } });
  fireEvent.click(screen.getByText("Runtime configuration (optional JSON)"));
  fireEvent.change(screen.getByRole("textbox", { name: "Runtime configuration (optional JSON)" }), { target: { value: '{"SERVICE_PORT":1234}' } });
  expect(screen.getByRole("button", { name: "Add" })).toBeDisabled();
  fireEvent.change(screen.getByRole("textbox", { name: "Runtime configuration (optional JSON)" }), { target: { value: '{"AGENT_EXEC":[]}' } });
  fireEvent.click(screen.getByRole("button", { name: "Add" }));
  await waitFor(() => expect(onCreated).toHaveBeenCalledWith("new-subject"));
  expect(invoke).toHaveBeenCalledWith("create_subject", { input: { connection: { base_url: "http://server", password: "token" }, name: null, instruction: "Investigate", config: { AGENT_EXEC: [] } } });
});
