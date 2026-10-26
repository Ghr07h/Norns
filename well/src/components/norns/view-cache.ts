export type NornsViewKind = "status" | "trace" | "visual";

export interface NornsViewCacheEntry {
  parameter: string;
  data: unknown;
  revision: number;
  sourceFilename: string | null;
}

const cache = new Map<string, NornsViewCacheEntry>();

export function nornsViewCacheKey(serverId: string, subjectId: string, kind: NornsViewKind): string {
  return JSON.stringify([serverId, subjectId, kind]);
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
