import { save } from "@tauri-apps/plugin-dialog";
import { writeTextFile } from "@tauri-apps/plugin-fs";
import type { NornsViewKind } from "./view-cache";

type ViewExport = {
  content: string;
  extension: "html" | "json";
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
): ViewExport {
  const extension = kind === "visual" ? "html" : "json";
  const content = kind === "visual" && typeof data === "string"
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

export async function saveNornsView(kind: NornsViewKind, data: unknown, sourceFilename: string | null): Promise<string | null> {
  const view = buildNornsViewExport(kind, data, sourceFilename);
  const path = await save({
    defaultPath: view.filename,
    filters: [{ name: view.extension === "html" ? "HTML" : "JSON", extensions: [view.extension] }],
  });
  if (!path) return null;
  await writeTextFile(path, view.content);
  return path;
}
