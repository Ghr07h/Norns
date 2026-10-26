import { describe, expect, it } from "vitest";
import { buildNornsViewExport } from "./view-export";

describe("Norns View export", () => {
  it("prefixes the exact server filename with well", () => {
    const view = buildNornsViewExport(
      "status",
      { generated_at: "2026-10-22T08:00:00Z" },
      "status-20261022T080000.123456789Z-a1b2c3d4.json",
      new Date("2000-01-01T00:00:00Z"),
      "ignored1",
    );
    expect(view.filename).toBe("well-status-20261022T080000.123456789Z-a1b2c3d4.json");
    expect(view.content).toBe("{\n  \"generated_at\": \"2026-10-22T08:00:00Z\"\n}\n");
  });

  it("uses the Norns timestamp shape when an older server omits the filename", () => {
    const view = buildNornsViewExport(
      "trace",
      { id: "exp" },
      null,
      new Date("2026-10-22T08:00:00.123Z"),
      "e5f6a7b8",
    );
    expect(view.filename).toBe("well-trace-20261022T080000.123000000Z-e5f6a7b8.json");
  });

  it("keeps Visual HTML unchanged", () => {
    const view = buildNornsViewExport(
      "visual",
      "<!doctype html><title>Norns</title>",
      "visual-20261022T080000.123456789Z-bbd20338.html",
    );
    expect(view.filename).toBe("well-visual-20261022T080000.123456789Z-bbd20338.html");
    expect(view.content).toBe("<!doctype html><title>Norns</title>");
  });
});
