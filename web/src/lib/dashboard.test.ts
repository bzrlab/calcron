import assert from "node:assert/strict";
import { DASHBOARD_LISTS, dueBucket, findById, jsonError, parseHash, waitingReason } from "./dashboard.ts";

assert.deepEqual(DASHBOARD_LISTS, [
  "apps", "schedules", "deliveries", "workflows", "history", "calendars",
  "tokens", "start_schedules", "workflow_versions",
]);

// Schedules group by when they fire. Local noon keeps "today" unambiguous.
const noon = new Date(2026, 8, 27, 12, 0, 0);
const at = (minutes: number) => new Date(noon.getTime() + minutes * 60_000).toISOString();
assert.equal(dueBucket(at(-1), "scheduled", noon), "overdue", "past and still scheduled");
assert.equal(dueBucket(at(30), "scheduled", noon), "hour", "within the next hour");
assert.equal(dueBucket(at(5 * 60), "scheduled", noon), "today", "later today");
assert.equal(dueBucket(at(13 * 60), "scheduled", noon), "later", "tomorrow");
assert.equal(dueBucket(at(-1), "delivered", noon), "done", "delivered is done regardless of time");
assert.equal(dueBucket(at(30), "cancelled", noon), "done", "cancelled is done regardless of time");

// Publish shows where JSON broke. Trailing comma: "}" sits at line 3, column 1.
assert.equal(jsonError('{"a": 1}'), null, "valid JSON has no error");
const broken = jsonError('{\n  "a": 1,\n}');
assert.ok(broken, "trailing comma is an error");
assert.equal(broken.line, 3);
assert.equal(broken.column, 1);
assert.match(broken.message, /^Line 3, column 1: /);

// Workflow rows say what a waiting instance is blocked on.
const base = { status: "running", waiting_event: null, correlation_key: null, wake_at: null };
assert.equal(
  waitingReason({ ...base, status: "waiting_signal", waiting_event: "payment.confirmed", correlation_key: "order-7" }, noon),
  "payment.confirmed / order-7",
);
assert.equal(waitingReason({ ...base, status: "waiting_time", wake_at: at(3 * 60) }, noon), "wakes in 3h");
assert.equal(waitingReason({ ...base, status: "waiting_ack" }, noon), "awaiting acknowledgement");
assert.equal(waitingReason({ ...base, status: "completed" }, noon), "");

// Drawer state lives in the hash so links can be shared.
assert.deepEqual(parseHash("#deliveries/dlv_1"), { view: "deliveries", id: "dlv_1" });
assert.deepEqual(parseHash("#overview"), { view: "overview", id: undefined });
assert.deepEqual(parseHash(""), { view: "", id: undefined });

// The palette opens whatever row owns a pasted ID.
const lists = {
  apps: [{ id: "app_1" }],
  deliveries: [{ id: "dlv_1" }],
  workflows: [{ id: "wf_1" }],
};
assert.deepEqual(findById(lists, "  dlv_1 "), { view: "deliveries", id: "dlv_1" });
assert.deepEqual(findById(lists, "wf_1"), { view: "workflows", id: "wf_1" });
assert.equal(findById(lists, "nope"), null);

console.log("dashboard verification passed");
