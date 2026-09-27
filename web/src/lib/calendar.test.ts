import assert from "node:assert/strict";
import { cycleOverride, dayOpen, days, fromYmd, hhmm, shift, wallTime, ymd, zoned } from "./calendar.ts";

// Sep 2026 starts on a Tuesday; the month grid is 6 full weeks from the Sunday before.
const month = days("month", new Date(2026, 8, 17));
assert.equal(month.length, 42);
assert.equal(ymd(month[0]), "2026-08-30");
assert.equal(ymd(month[41]), "2026-10-10");

const week = days("week", new Date(2026, 8, 30));
assert.deepEqual([ymd(week[0]), ymd(week[6])], ["2026-09-27", "2026-10-03"]);
assert.deepEqual(days("day", new Date(2026, 8, 30, 15)).map(ymd), ["2026-09-30"]);

assert.equal(ymd(shift("month", new Date(2026, 0, 31), 1)), "2026-02-01", "month step lands inside the next month");
assert.equal(ymd(shift("week", new Date(2026, 8, 30), -1)), "2026-09-23");
assert.equal(ymd(shift("day", new Date(2026, 11, 31), 1)), "2027-01-01");

// Sun-Thu business week with a closed holiday and an extra open Saturday.
const def = { timezone: "Asia/Dhaka", weekdays: [0, 1, 2, 3, 4], overrides: { "2026-09-28": false, "2026-10-03": true } };
assert.deepEqual(dayOpen(def, "2026-09-27"), { open: true, override: false }, "Sunday is a working day");
assert.deepEqual(dayOpen(def, "2026-10-02"), { open: false, override: false }, "Friday is off");
assert.deepEqual(dayOpen(def, "2026-09-28"), { open: false, override: true }, "holiday closes a Monday");
assert.deepEqual(dayOpen(def, "2026-10-03"), { open: true, override: true }, "override opens a Saturday");

// Clicking a day flips it against the weekday rule, clicking again restores the rule.
assert.deepEqual(cycleOverride(def, "2026-09-29"), { ...def.overrides, "2026-09-29": false });
assert.deepEqual(cycleOverride(def, "2026-10-02"), { ...def.overrides, "2026-10-02": true });
assert.deepEqual(cycleOverride(def, "2026-09-28"), { "2026-10-03": true });

// An instant lands on a different day and minute depending on the display timezone.
const instant = new Date("2026-09-27T20:30:00Z");
assert.deepEqual(zoned(instant, "UTC"), { day: "2026-09-27", minutes: 20 * 60 + 30 });
assert.deepEqual(zoned(instant, "Asia/Dhaka"), { day: "2026-09-28", minutes: 2 * 60 + 30 });
assert.deepEqual(zoned(instant, "America/New_York"), { day: "2026-09-27", minutes: 16 * 60 + 30 });

// A dropped wall-clock slot converts back to the same instant, including on DST days.
assert.equal(wallTime("2026-09-28", 2 * 60 + 30, "Asia/Dhaka").toISOString(), instant.toISOString());
assert.equal(wallTime("2025-03-09", 12 * 60, "America/New_York").toISOString(), "2025-03-09T16:00:00.000Z", "after spring-forward is EDT");
assert.equal(wallTime("2025-03-08", 12 * 60, "America/New_York").toISOString(), "2025-03-08T17:00:00.000Z", "before spring-forward is EST");
assert.equal(ymd(fromYmd("2026-02-28")), "2026-02-28");
assert.equal(hhmm(9 * 60 + 5), "09:05");

console.log("calendar verification passed");
