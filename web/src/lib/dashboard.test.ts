import assert from "node:assert/strict";
import { DASHBOARD_LISTS } from "./dashboard.ts";

assert.deepEqual(DASHBOARD_LISTS, ["apps", "schedules", "deliveries", "workflows", "history", "calendars"]);

console.log("dashboard catalog verification passed");
