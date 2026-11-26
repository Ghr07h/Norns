export type NornsViewKind = "status" | "trace" | "visual" | "cognition" | "exploration";

export interface NornsViewCacheEntry {
  parameter: string;
  data: unknown;
  revision: number;
  sourceFilename: string | null;
}

const cache = new Map<string, NornsViewCacheEntry>();

export function nornsViewCacheKey(serverId: string, subjectId: string, kind: NornsViewKind, format = "md"): string {
  return JSON.stringify([serverId, subjectId, kind, format]);
}

export function readNornsViewCache(key: string): NornsViewCacheEntry | undefined {
  return cache.get(key);
}

export function writeNornsViewCache(key: string, entry: NornsViewCacheEntry): void {
  cache.set(key, entry);
}

export function clearNornsViewCache(): void {
  cache.clear();
}
