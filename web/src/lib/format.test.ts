// Minimal check for timeAgo/fmt — the branch that used to render "-30s ago"
// for future deadlines. Run: node --experimental-strip-types web/src/lib/format.test.ts
import assert from "node:assert/strict";
import { timeAgo } from "./format.ts";

const now = Date.now();
const at = (ms: number) => new Date(now + ms).toISOString();

assert.equal(timeAgo(at(30_000)), "in 30s", "future under a minute");
assert.equal(timeAgo(at(30 * 60_000)), "in 30m", "future under an hour");
assert.equal(timeAgo(at(3 * 3600_000)), "in 3h", "future under a day");
assert.equal(timeAgo(at(2 * 86400_000)), "in 2d", "future beyond a day");
assert.equal(timeAgo(at(-5_000)), "5s ago", "past under a minute");
assert.equal(timeAgo(at(-90 * 60_000)), "2h ago", "past beyond an hour");
assert.equal(timeAgo(undefined), "—", "missing value");
assert.equal(timeAgo("not-a-date"), "not-a-date", "unparseable value");

for (const [label, ms] of [["in 30s", 30_000], ["in 30m", 30 * 60_000], ["in 2d", 2 * 86400_000]] as const) {
  assert.ok(!timeAgo(at(ms)).includes("-"), `${label} must not contain a negative sign`);
}

console.log("format verification passed");
