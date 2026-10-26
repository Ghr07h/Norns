import { afterEach, expect, it } from "vitest";
import type { PublicEvent } from "../api/dto";
import { cacheEvents, clearAllCaches, clearServerCache, readCachedEventsAfter, readCachedEventsBefore, readEventCacheState, readLatestCachedEvents, seqSortKey } from "./event-db";

afterEach(async () => clearAllCaches());

function event(seq: number): PublicEvent {
  return { seq: String(seq), type: "agent.text", timestamp: "2026-10-14T00:00:00Z", runtime_id: null, source: "agent", payload: { text: `event ${seq}` } };
}

it("orders the full uint64 range without Number conversion", async () => {
  expect(seqSortKey("2") < seqSortKey("9007199254740993")).toBe(true);
  expect(seqSortKey("18446744073709551615")).toBe("18446744073709551615");
  expect(() => seqSortKey("18446744073709551616")).toThrow();
});

it("isolates event scopes and clears one server without touching another", async () => {
  await cacheEvents("server-a", "subject", [event(1), event(2)]);
  await cacheEvents("server-b", "subject", [event(3)]);
  expect((await readLatestCachedEvents("server-a", "subject", 10)).map((item) => item.seq)).toEqual(["1", "2"]);
  expect((await readLatestCachedEvents("server-b", "subject", 10)).map((item) => item.seq)).toEqual(["3"]);
  await clearServerCache("server-a");
  expect(await readLatestCachedEvents("server-a", "subject", 10)).toEqual([]);
  expect((await readLatestCachedEvents("server-b", "subject", 10)).map((item) => item.seq)).toEqual(["3"]);
});

it("reads cached events in bounded windows and retains cursors", async () => {
  await cacheEvents("server", "subject", Array.from({ length: 5_001 }, (_, index) => event(index + 1)));
  await cacheEvents("server", "subject", [], { syncCursor: "sync", olderCursor: "older" });
  expect((await readLatestCachedEvents("server", "subject", 3)).map((item) => item.seq)).toEqual(["4999", "5000", "5001"]);
  expect((await readCachedEventsBefore("server", "subject", "4999", 2)).map((item) => item.seq)).toEqual(["4997", "4998"]);
  expect((await readCachedEventsAfter("server", "subject", "4999", 2)).map((item) => item.seq)).toEqual(["5000", "5001"]);
  expect(await readEventCacheState("server", "subject")).toMatchObject({ eventCount: 5_001, syncCursor: "sync", olderCursor: "older" });
});
