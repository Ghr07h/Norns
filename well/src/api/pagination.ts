export interface CursorPage<T> {
  items: T[];
  nextCursor: string | null;
}

export async function collectCursorPages<T>(loader: (cursor: string | null) => Promise<CursorPage<T>>): Promise<T[]> {
  const result: T[] = [];
  let cursor: string | null = null;
  let restarted = false;
  for (;;) {
    try {
      const page = await loader(cursor);
      result.push(...page.items);
      if (!page.nextCursor) return result;
      cursor = page.nextCursor;
    } catch (error) {
      const code = typeof error === "object" && error && "code" in error ? (error as { code?: string }).code : undefined;
      if (code === "INVALID_CURSOR" && !restarted) {
        restarted = true;
        cursor = null;
        result.length = 0;
        continue;
      }
      throw error;
    }
  }
}
