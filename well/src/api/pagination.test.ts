import { collectCursorPages } from "./pagination";

describe("collectCursorPages", () => {
  it("publishes only a complete collection", async () => {
    const pages = new Map<string | null, { items: number[]; nextCursor: string | null }>([[null, { items: [1, 2], nextCursor: "next" }], ["next", { items: [3], nextCursor: null }]]);
    await expect(collectCursorPages(async (cursor) => pages.get(cursor)!)).resolves.toEqual([1, 2, 3]);
  });

  it("restarts one invalid cursor from the first page", async () => {
    let first = true;
    const result = await collectCursorPages(async (cursor) => {
      if (cursor === null) return { items: [1], nextCursor: "next" };
      if (first) { first = false; throw { code: "INVALID_CURSOR" }; }
      return { items: [2], nextCursor: null };
    });
    expect(result).toEqual([1, 2]);
  });
});
