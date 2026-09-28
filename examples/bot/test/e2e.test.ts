import assert from "node:assert/strict";
import test from "node:test";
import { Calcron, type EventName } from "@bzrlab/calcron";
import { Admin, verifyKey, type Events, type Workflows } from "../src/calcron.ts";
import { overrideDate } from "../src/features/ops.ts";
import { CALENDAR, loadCalendar, provision } from "../src/provision.ts";

const url = process.env.CALCRON_E2E_URL;
const adminToken = process.env.CALCRON_E2E_ADMIN_TOKEN;

test("bot workflows run end to end against Calcron", { skip: !url || !adminToken ? "set CALCRON_E2E_URL and CALCRON_E2E_ADMIN_TOKEN" : false, timeout: 30_000 }, async t => {
  const admin = await Admin.connect(url, adminToken);
  t.after(() => admin.close());
  const name = `bot-e2e-${Date.now()}`;
  const app = await admin.call<{ applicationId: string; token: string }>("app.create", { name, namespace: name });
  const options = { appId: app.applicationId, standupChannelId: "c1", timezone: "Asia/Dhaka", weekdays: [0, 1, 2, 3, 4], standupTime: "10:00", standupWindow: "1s", missedPolicy: "skip" as const };
  const first = await provision(admin, options);
  assert.deepEqual(first.versions, { "member-verification": 1, "daily-standup": 1 });

  // Holiday overrides added from Discord survive re-provisioning.
  const calendar = (await loadCalendar(admin, app.applicationId))!;
  await admin.call("calendar.set", { applicationId: app.applicationId, name: CALENDAR, data: { ...calendar, overrides: { "2026-12-16": false } } });
  // Unchanged definitions are not re-published; a changed one gets a new version.
  const second = await provision(admin, { ...options, standupWindow: "2s" });
  assert.deepEqual(second.versions, { "member-verification": 1, "daily-standup": 2 });
  assert.deepEqual((await loadCalendar(admin, app.applicationId))?.overrides, { "2026-12-16": false });

  // Declaring the next standup day a holiday moves the start schedule past it; clearing restores it.
  const day = new Date(second.nextStandup).toLocaleDateString("en-CA", { timeZone: options.timezone });
  const moved = await overrideDate(admin, app.applicationId, day, "off");
  assert.ok(Date.parse(moved["daily-standup"]) > Date.parse(second.nextStandup), `${moved["daily-standup"]} should be after ${second.nextStandup}`);
  assert.equal(Date.parse((await overrideDate(admin, app.applicationId, day, "default"))["daily-standup"]), Date.parse(second.nextStandup));

  const bot = new Calcron<Events, Workflows>(url!, app.token);
  const inbox = new Map<string, unknown[]>();
  const waiters = new Map<string, () => void>();
  const events: EventName<Events>[] = ["verify.prompt", "verify.grant", "verify.reject", "standup.open", "standup.close"];
  for (const event of events) bot.on(event, async d => { await d.ack(); inbox.set(event, [...(inbox.get(event) ?? []), d.data]); waiters.get(event)?.(); });
  const next = async <N extends EventName<Events>>(event: N, count = 1) => {
    while ((inbox.get(event)?.length ?? 0) < count) await new Promise<void>(resolve => waiters.set(event, resolve));
    return inbox.get(event)!.slice(0, count) as Events[N][];
  };
  await bot.connect();
  t.after(() => bot.close());

  const alice = { guildId: "g1", userId: "alice", joinedAt: "1" };
  const bob = { guildId: "g1", userId: "bob", joinedAt: "1" };
  const bobRejoined = { ...bob, joinedAt: "2" };
  await bot.start("member-verification", "alice:start", alice);
  await bot.start("member-verification", "bob:start", bob);
  await bot.start("member-verification", "bob:rejoin:start", bobRejoined);
  assert.deepEqual((await next("verify.prompt", 3)).map(d => `${d.userId}@${d.joinedAt}`).sort(), ["alice@1", "bob@1", "bob@2"]);

  const approved = await bot.signal("member.verified", verifyKey(bob), "bob:verdict", { ...bob, approved: true, by: "bob", reason: "accepted" });
  assert.equal(approved.matched, 1);
  assert.deepEqual(await next("verify.grant"), [{ ...bob, approved: true, by: "bob", reason: "accepted" }]);

  const rejected = await bot.signal("member.verified", verifyKey(alice), "alice:verdict", { ...alice, approved: false, by: "calcron", reason: "timeout" });
  assert.equal(rejected.matched, 1);
  assert.equal((await next("verify.reject"))[0].userId, "alice");
  const left = await bot.signal("member.verified", verifyKey(bobRejoined), "bob:left", { ...bobRejoined, approved: false, by: "discord", reason: "left" });
  assert.equal(left.matched, 1);
  assert.equal((await next("verify.reject", 2))[1].joinedAt, "2");
  assert.equal(inbox.get("verify.grant")!.length, 1);

  const adHoc = { channelId: "c1", name: "Ad-hoc standup · e2e" };
  await bot.start("daily-standup", "standup:e2e", adHoc);
  assert.deepEqual(await next("standup.open"), [adHoc]);
  assert.deepEqual(await next("standup.close"), [adHoc]);
});
