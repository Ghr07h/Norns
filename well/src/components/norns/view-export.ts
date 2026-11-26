import { save } from "@tauri-apps/plugin-dialog";
import { writeTextFile } from "@tauri-apps/plugin-fs";
import type { NornsViewKind } from "./view-cache";

type ViewExport = {
  content: string;
  extension: "html" | "json" | "md" | "yaml";
  filename: string;
};

function timestamp(date: Date): string {
  return date.toISOString()
    .replace(/[-:]/g, "")
    .replace(/\.(\d{3})Z$/, ".$1000000Z");
}

function randomToken(): string {
  return crypto.randomUUID().replaceAll("-", "").slice(0, 8);
}

export function buildNornsViewExport(
  kind: NornsViewKind,
  data: unknown,
  sourceFilename: string | null = null,
  date = new Date(),
  token = randomToken(),
  format: "json" | "md" | "yaml" = "json",
): ViewExport {
  const extension = kind === "visual" ? "html" : ["cognition", "exploration"].includes(kind) ? "json" : format;
  const content = typeof data === "string"
    ? data
    : `${JSON.stringify(data, null, 2)}\n`;
  const validSource = sourceFilename
    && sourceFilename.length <= 240
    && sourceFilename.startsWith(`${kind}-`)
    && sourceFilename.endsWith(`.${extension}`)
    && /^[A-Za-z0-9._-]+$/.test(sourceFilename);
  return {
    content,
    extension,
    filename: validSource
      ? `well-${sourceFilename}`
      : `well-${kind}-${timestamp(date)}-${token}.${extension}`,
  };
}

export async function saveNornsView(kind: NornsViewKind, data: unknown, sourceFilename: string | null, format: "json" | "md" | "yaml" = "json"): Promise<string | null> {
  const view = buildNornsViewExport(kind, data, sourceFilename, new Date(), randomToken(), format);
  const path = await save({
    defaultPath: view.filename,
    filters: [{ name: view.extension.toUpperCase(), extensions: [view.extension] }],
  });
  if (!path) return null;
  await writeTextFile(path, view.content);
  return path;
}
