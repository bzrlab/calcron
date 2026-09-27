import { execFileSync } from "node:child_process";

const base = process.env.CALCRON_URL ?? "http://127.0.0.1:8080";
const second = process.env.CALCRON_SECOND_URL;
const adminToken = process.env.CALCRON_ADMIN_TOKEN ?? "change-me";
const composeProject = process.env.CALCRON_E2E_PROJECT;
const composeFile = process.env.CALCRON_E2E_COMPOSE_FILE ?? "compose.yaml";
const composeFiles = ["--file", composeFile, ...(process.env.CALCRON_E2E_OVERRIDE_FILE ? ["--file", process.env.CALCRON_E2E_OVERRIDE_FILE] : [])];
const wsURL = url => url.replace(/^http/, "ws") + "/ws";
const sleep = ms => new Promise(ok => setTimeout(ok, ms));
class Client {
  constructor(url, token) { this.url = url; this.token = token; this.n = 0; this.pending = new Map(); this.events = []; this.waiters = []; }
  async connect() { this.ws = new WebSocket(wsURL(this.url)); await new Promise((ok, fail) => { this.ws.onopen = ok; this.ws.onerror = () => fail(new Error("WebSocket failed")); }); this.ws.onmessage = e => this.receive(JSON.parse(e.data)); await this.request({ op: "auth", token: this.token }); return this; }
  receive(m) { if (m.id && this.pending.has(m.id)) { const p = this.pending.get(m.id); this.pending.delete(m.id); m.ok ? p.resolve(m.data) : p.reject(new Error(m.error)); return; } if (m.op !== "delivery") return; const i = this.waiters.findIndex(w => w.match(m)); if (i >= 0) { const w = this.waiters.splice(i, 1)[0]; clearTimeout(w.timer); w.resolve(m); } else this.events.push(m); }
  request(body) { return new Promise((resolve, reject) => { const id = String(++this.n); this.pending.set(id, { resolve, reject }); this.ws.send(JSON.stringify({ id, ...body })); }); }
  event(match = () => true, ms = 5_000) { const i = this.events.findIndex(match); if (i >= 0) return Promise.resolve(this.events.splice(i, 1)[0]); return new Promise((resolve, reject) => { const w = { match, resolve, reject, timer: setTimeout(() => { this.waiters = this.waiters.filter(x => x !== w); reject(new Error("delivery timeout")); }, ms) }; this.waiters.push(w); }); }
  close() { this.ws.close(); }
}
const key = label => `e2e:${label}:${Date.now()}:${Math.random()}`;
const assert = (v, m) => { if (!v) throw new Error(m); };
const noEvent = async (c, event, ms = 400) => { try { await c.event(x => x.event === event, ms); throw new Error(`unexpected ${event}`); } catch (e) { if (e.message !== "delivery timeout") throw e; } };
const connect = (url, token) => new Client(url, token).connect();
const planNodes = plan => [plan, ...(plan.Plans ?? []).flatMap(planNodes)];
const queryPlan = sql => {
  if (!composeProject) return null;
  const raw = execFileSync("docker", ["compose", "--project-name", composeProject, ...composeFiles, "exec", "-T", "-e", "PGOPTIONS=-c enable_seqscan=off", "postgres", "psql", "-U", "calcron", "-d", "calcron", "-X", "-q", "-t", "-A", "-c", `explain (format json) ${sql}`], { encoding: "utf8" });
  return JSON.parse(raw)[0].Plan;
};
const assertPlanUses = (label, sql, index) => {
  const plan = queryPlan(sql);
  if (!plan) return;
  assert(planNodes(plan).some(node => node["Index Name"] === index), `${label} did not use ${index}: ${JSON.stringify(plan)}`);
};

if (!(await fetch(base + "/health")).ok) throw new Error("health failed");
const admin = await connect(base, adminToken);
const created = await admin.request({ op: "app.create", name: "e2e", namespace: "e2e-" + Date.now() });
const app = await connect(base, created.token);

await app.request({ op: "schedule.set", idempotencyKey: key("replace-a"), key: "replace", event: "replace.old", after: "1s" });
await app.request({ op: "schedule.set", idempotencyKey: key("replace-b"), key: "replace", event: "replace.new", after: "20ms" });
const replaced = await app.event(x => x.event === "replace.new"); assert(replaced.event === "replace.new", "replace failed"); await app.request({ op: "delivery.ack", deliveryId: replaced.deliveryId, idempotencyKey: key("replace-ack") }); await noEvent(app, "replace.old", 1_100);
await app.request({ op: "schedule.set", idempotencyKey: key("cancel-set"), key: "cancel", event: "cancelled", after: "20ms" }); await app.request({ op: "schedule.cancel", idempotencyKey: key("cancel"), key: "cancel" }); await noEvent(app, "cancelled");
const before = Date.now(); await app.request({ op: "schedule.set", idempotencyKey: key("extend-set"), key: "extend", event: "extended", after: "20ms" }); await app.request({ op: "schedule.extend", idempotencyKey: key("extend"), key: "extend", by: "300ms" }); const extended = await app.event(x => x.event === "extended"); assert(Date.now() - before >= 250, "extend fired early"); await app.request({ op: "delivery.ack", deliveryId: extended.deliveryId, idempotencyKey: key("extend-ack") });
assert((await app.request({ op: "schedule.throttle", idempotencyKey: key("throttle-a"), key: "throttle", event: "throttled", cooldown: "1s" })).triggered, "throttle first failed"); const throttled = await app.event(x => x.event === "throttled"); await app.request({ op: "delivery.ack", deliveryId: throttled.deliveryId, idempotencyKey: key("throttle-ack") }); assert(!(await app.request({ op: "schedule.throttle", idempotencyKey: key("throttle-b"), key: "throttle", event: "throttled", cooldown: "1s" })).triggered, "throttle repeated");
await app.request({ op: "schedule.set", idempotencyKey: key("chain-set"), key: "chain", event: "chain.parent", after: "20ms", chain: { key: "chain.next", event: "chain.child", after: "20ms" } }); const parent = await app.event(x => x.event === "chain.parent"); await app.request({ op: "delivery.ack", deliveryId: parent.deliveryId, idempotencyKey: key("chain-ack") }); const child = await app.event(x => x.event === "chain.child"); await app.request({ op: "delivery.ack", deliveryId: child.deliveryId, idempotencyKey: key("child-ack") });

const offlineCreated = await admin.request({ op: "app.create", name: "offline", namespace: "offline-" + Date.now() }); const offline = await connect(base, offlineCreated.token); await offline.request({ op: "schedule.set", idempotencyKey: key("offline-set"), key: "offline", event: "offline.event", after: "50ms" }); offline.close(); await sleep(300); const back = await connect(base, offlineCreated.token); const offlineEvent = await back.event(x => x.event === "offline.event"); await back.request({ op: "delivery.ack", deliveryId: offlineEvent.deliveryId, idempotencyKey: key("offline-ack") });
await app.request({ op: "schedule.set", idempotencyKey: key("blocked-set"), key: "blocked", event: "blocked.event", after: "20ms" }); const blockedFirst = await app.event(x => x.event === "blocked.event"); for (let i = 0; i < 4; i++) await app.event(x => x.deliveryId === blockedFirst.deliveryId, 5_000); await sleep(1_000); const deliveries = await admin.request({ op: "dashboard.list", name: "deliveries" }); const blocked = deliveries.find(x => x.id === blockedFirst.deliveryId); assert(blocked?.status === "blocked", "delivery did not block"); assert((await admin.request({ op: "delivery.replay", deliveryId: blockedFirst.deliveryId })).replayed, "replay failed"); const replayed = await app.event(x => x.deliveryId === blockedFirst.deliveryId); await app.request({ op: "delivery.ack", deliveryId: replayed.deliveryId, idempotencyKey: key("replay-ack") });
await app.request({ op: "schedule.set", idempotencyKey: key("recovery-set"), key: "recovery", event: "recovery.event", after: "20ms" }); const recovery = await app.event(x => x.event === "recovery.event"); assert((await admin.request({ op: "delivery.cancel", deliveryId: recovery.deliveryId })).cancelled, "delivery cancel failed"); await noEvent(app, "recovery.event", 1_100);

const definition = { initial: "time", states: { time: { type: "wait_time", after: "20ms", next: "wait" }, wait: { type: "wait_signal", event: "confirm", correlationKey: "order-1", next: "branch" }, branch: { type: "branch", when: "state.signal.ok == true", true: "emit", false: "end" }, emit: { type: "emit", target: created.applicationId, event: "workflow.done", next: "end" }, end: { type: "end" } } };
await admin.request({ op: "workflow.publish", applicationId: created.applicationId, name: "e2e-workflow", data: definition }); await app.request({ op: "workflow.start", name: "e2e-workflow", idempotencyKey: key("workflow-start") }); await sleep(350); await app.request({ op: "signal", event: "confirm", correlationKey: "order-1", data: { ok: true }, idempotencyKey: key("signal-true") }); const workflowEvent = await app.event(x => x.event === "workflow.done"); await app.request({ op: "delivery.ack", deliveryId: workflowEvent.deliveryId, idempotencyKey: key("workflow-ack") }); await app.request({ op: "workflow.start", name: "e2e-workflow", idempotencyKey: key("workflow-false-start") }); await sleep(350); await app.request({ op: "signal", event: "confirm", correlationKey: "order-1", data: { ok: false }, idempotencyKey: key("signal-false") }); await noEvent(app, "workflow.done", 600);

await admin.request({ op: "calendar.set", applicationId: created.applicationId, name: "dst", data: { timezone: "America/New_York", weekdays: [1, 2, 3, 4, 5], overrides: { "2026-03-08": true } } });
const dst = await admin.request({ op: "calendar.next", applicationId: created.applicationId, calendar: "dst", localTime: "09:00", at: "2026-03-08T00:00:00Z" }); assert(dst.nextAt === "2026-03-08T13:00:00Z", "DST override failed");
const recurring = { initial: "emit", states: { emit: { type: "emit", target: created.applicationId, event: "recurring.fire", next: "end" }, end: { type: "end" } } };
await admin.request({ op: "workflow.publish", applicationId: created.applicationId, name: "e2e-recurring", data: recurring }); await admin.request({ op: "calendar.set", applicationId: created.applicationId, name: "all-days", data: { timezone: "UTC", weekdays: [0, 1, 2, 3, 4, 5, 6] } });
const now = Date.now(); const pastMinute = new Date(now - 60_000); pastMinute.setUTCSeconds(0, 0); const clock = pastMinute.toISOString().slice(11, 16); const catchUpAt = new Date(now - 172_800_000); catchUpAt.setUTCHours(0, 0, 0, 0);
await admin.request({ op: "start-schedule.set", applicationId: created.applicationId, name: "start-skip", workflow: "e2e-recurring", calendar: "all-days", localTime: clock, missedPolicy: "skip", at: pastMinute.toISOString() });
await admin.request({ op: "start-schedule.set", applicationId: created.applicationId, name: "start-once", workflow: "e2e-recurring", calendar: "all-days", localTime: clock, missedPolicy: "run_once_late", at: pastMinute.toISOString() });
await admin.request({ op: "start-schedule.set", applicationId: created.applicationId, name: "start-catch", workflow: "e2e-recurring", calendar: "all-days", localTime: "00:00", missedPolicy: "catch_up", at: catchUpAt.toISOString() });
const recurringIDs = new Set(); while (recurringIDs.size < 3) { const event = await app.event(x => x.event === "recurring.fire", 10_000); if (!recurringIDs.has(event.deliveryId)) { recurringIDs.add(event.deliveryId); await app.request({ op: "delivery.ack", deliveryId: event.deliveryId, idempotencyKey: key(`recurring-ack-${recurringIDs.size}`) }); } } assert(recurringIDs.size === 3, "missed policy recovery failed");

const rotated = await admin.request({ op: "app.token.rotate", applicationId: created.applicationId }); const rotatedClient = await connect(base, rotated.token); rotatedClient.close(); assert((await admin.request({ op: "app.token.revoke", tokenId: rotated.tokenId })).revoked, "revoke failed"); try { await connect(base, rotated.token); throw new Error("revoked token authenticated"); } catch (e) { if (e.message === "revoked token authenticated") throw e; }
for (const name of ["apps", "schedules", "deliveries", "workflows", "calendars", "history"]) assert(Array.isArray(await admin.request({ op: "dashboard.list", name })), `dashboard ${name} failed`);

if (second) { const multi = await admin.request({ op: "app.create", name: "multi", namespace: "multi-" + Date.now() }); const one = await connect(base, multi.token); const two = await connect(second, multi.token); const hits = []; await one.request({ op: "schedule.set", idempotencyKey: key("multi"), key: "multi", event: "multi.event", after: "50ms" }); const first = one.event(x => x.event === "multi.event", 700).then(x => { hits.push(x); return { x, c: one }; }); const other = two.event(x => x.event === "multi.event", 700).then(x => { hits.push(x); return { x, c: two }; }); const winner = await Promise.race([first, other]); await winner.c.request({ op: "delivery.ack", deliveryId: winner.x.deliveryId, idempotencyKey: key("multi-ack") }); await Promise.allSettled([first, other]); await sleep(100); assert(hits.filter(x => x.deliveryId === winner.x.deliveryId).length === 1 && one.events.concat(two.events).filter(x => x.deliveryId === winner.x.deliveryId).length === 0, "multi-instance lease contention delivered twice"); one.close(); two.close(); }
assertPlanUses("due schedules", "select id,application_id,event,payload from schedules where status='scheduled' and run_at<=now() order by run_at limit 100 for update skip locked", "schedules_due_idx");
assertPlanUses("pending deliveries", "select id,application_id,coalesce(schedule_id,''),event,payload,attempts from deliveries where status='pending' and next_attempt_at<=now() order by next_attempt_at limit 100", "deliveries_pending_idx");
assertPlanUses("dashboard history", "select application_id,subject_type,subject_id,event,data,created_at from history order by id desc limit 100", "history_pkey");
admin.close(); app.close(); back.close(); console.log("e2e passed");
