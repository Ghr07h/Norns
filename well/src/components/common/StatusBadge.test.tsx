import { render, screen } from "@testing-library/react";
import { beforeAll, expect, it } from "vitest";
import i18n from "../../i18n";
import { StatusBadge } from "./StatusBadge";

beforeAll(async () => {
  await i18n.changeLanguage("zh");
});

it("uses semantic colors for normal, advisory, and blocking states", () => {
  render(<>
    <StatusBadge value="online" />
    <StatusBadge value="duplicate_identity" />
    <StatusBadge value="identity_changed" />
  </>);

  expect(screen.getByText("在线")).toHaveClass("status-ok");
  expect(screen.getByText("重复的 Server Root 记录")).toHaveClass("status-warning");
  expect(screen.getByText("目标服务已改变")).toHaveClass("status-danger");
});
