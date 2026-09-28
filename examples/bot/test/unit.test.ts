import assert from "node:assert/strict";
import test from "node:test";
import { parseWhen } from "../src/calcron.ts";
import { pickWinners } from "../src/features/giveaways.ts";

test("parseWhen maps durations to after and dates to UTC at", () => {
  const now = Date.parse("2026-09-28T00:00:00Z");
  assert.deepEqual(parseWhen("10m", now), { after: "10m" });
  assert.deepEqual(parseWhen("1h 30m", now), { after: "1h30m" });
  assert.deepEqual(parseWhen("2026-10-01T09:00+06:00", now), { at: "2026-10-01T03:00:00Z" });
  assert.deepEqual(parseWhen(String(now / 1000 + 3600), now), { at: "2026-09-28T01:00:00Z" });
  assert.throws(() => parseWhen("2026-09-27T00:00:00Z", now), /past/);
  assert.throws(() => parseWhen("7d", now), /duration/);
});

test("pickWinners draws distinct entrants and never more than entered", () => {
  const winners = pickWinners(["a", "b", "b", "c", "d"], 3);
  assert.equal(winners.length, 3);
  assert.equal(new Set(winners).size, 3);
  assert.deepEqual(pickWinners(["a", "a"], 5), ["a"]);
  assert.deepEqual(pickWinners([], 2), []);
});
