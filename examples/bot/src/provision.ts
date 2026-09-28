import { fileURLToPath } from "node:url";
import { Admin, env, isDuration } from "./calcron.ts";

export type ProvisionOptions = {
  appId: string;
  standupChannelId: string;
  timezone: string;
  weekdays: number[];
  standupTime: string;
  standupWindow: string;
  missedPolicy: "skip" | "run_once_late" | "catch_up";
};
type Calendar = { timezone: string; weekdays: number[]; overrides?: Record<string, boolean> };
export const CALENDAR = "workdays";

export function workflows(appId: string, standupWindow: string) {
  const emit = (event: string, next: string, dataExpr = "input") => ({ type: "emit", target: appId, event, dataExpr, next });
  return {
    "member-verification": {
      initial: "prompt",
      states: {
        prompt: emit("verify.prompt", "wait"),
        wait: { type: "wait_signal", event: "member.verified", correlationKeyExpr: "'verify:' + input.guildId + ':' + input.userId + ':' + input.joinedAt", next: "decide" },
        decide: { type: "branch", when: "state.signal.approved == true", true: "grant", false: "reject" },
        grant: emit("verify.grant", "done", "state.signal"),
        reject: emit("verify.reject", "done", "state.signal"),
        done: { type: "end" },
      },
    },
    "daily-standup": {
      initial: "open",
      states: {
        open: emit("standup.open", "window"),
        window: { type: "wait_time", after: standupWindow, next: "close" },
        close: emit("standup.close", "done"),
        done: { type: "end" },
      },
    },
  };
}

export async function loadCalendar(admin: Admin, appId: string) {
  const rows = await admin.call<{ name: string; definition: Calendar }[]>("dashboard.list", { name: "calendars", applicationId: appId });
  return rows.find(row => row.name === CALENDAR)?.definition;
}

/** Drops empty fields and sorts keys, so a stored definition compares equal to the one that produced it. */
const canonical = (value: unknown): unknown => Array.isArray(value) ? value.map(canonical)
  : value && typeof value === "object" ? Object.fromEntries(Object.entries(value).filter(([, v]) => v !== "" && v != null).sort(([a], [b]) => a.localeCompare(b)).map(([k, v]) => [k, canonical(v)]))
  : value;
const same = (a: unknown, b: unknown) => JSON.stringify(canonical(a)) === JSON.stringify(canonical(b));

/** Publishes changed workflow definitions and upserts the calendar and start schedule. Holiday overrides survive re-provisioning. */
export async function provision(admin: Admin, o: ProvisionOptions) {
  if (!isDuration(o.standupWindow)) throw new Error("STANDUP_WINDOW must be a Go duration");
  const latest = await admin.call<{ name: string; version: number; definition: unknown }[]>("dashboard.list", { name: "workflow_versions", applicationId: o.appId });
  const versions: Record<string, number> = {};
  for (const [name, data] of Object.entries(workflows(o.appId, o.standupWindow))) {
    const current = latest.find(row => row.name === name);
    versions[name] = current && same(current.definition, data) ? current.version
      : (await admin.call<{ version: number }>("workflow.publish", { applicationId: o.appId, name, data })).version;
  }
  const overrides = (await loadCalendar(admin, o.appId))?.overrides ?? {};
  await admin.call("calendar.set", { applicationId: o.appId, name: CALENDAR, data: { timezone: o.timezone, weekdays: o.weekdays, overrides } });
  const start = await admin.call<{ nextAt: string }>("start-schedule.set", {
    applicationId: o.appId, name: "daily-standup", workflow: "daily-standup", calendar: CALENDAR,
    localTime: o.standupTime, missedPolicy: o.missedPolicy, data: { channelId: o.standupChannelId },
  });
  return { versions, nextStandup: start.nextAt };
}

export function optionsFromEnv(): ProvisionOptions {
  const missedPolicy = env("STANDUP_MISSED_POLICY", "run_once_late");
  if (missedPolicy !== "skip" && missedPolicy !== "run_once_late" && missedPolicy !== "catch_up") throw new Error("STANDUP_MISSED_POLICY must be skip, run_once_late or catch_up");
  return {
    appId: env("CALCRON_APP_ID"),
    standupChannelId: env("STANDUP_CHANNEL_ID"),
    timezone: env("CALENDAR_TIMEZONE", "Asia/Dhaka"),
    weekdays: env("CALENDAR_WEEKDAYS", "0,1,2,3,4").split(",").map(Number),
    standupTime: env("STANDUP_TIME", "10:00"),
    standupWindow: env("STANDUP_WINDOW", "4h"),
    missedPolicy,
  };
}

async function main([command = "all", arg]: string[]) {
  const admin = await Admin.connect();
  try {
    if (command === "app") {
      const name = arg ?? "discord-bot";
      const app = await admin.call<{ applicationId: string; token: string }>("app.create", { name, namespace: name });
      console.log(`CALCRON_APP_ID=${app.applicationId}\nCALCRON_TOKEN=${app.token}\n\nThe token is shown once. Store it now.`);
    } else if (command === "rotate") {
      const next = await admin.call<{ tokenId: string; token: string }>("app.token.rotate", { applicationId: env("CALCRON_APP_ID") });
      const old = process.env.CALCRON_TOKEN?.split("_")[1] ?? "<old token id>";
      console.log(`CALCRON_TOKEN=${next.token}\n\nDeploy it to every bot instance, then run: npm run provision -- revoke ${old}`);
    } else if (command === "revoke") {
      if (!arg) throw new Error("usage: npm run provision -- revoke <tokenId>");
      console.log(await admin.call("app.token.revoke", { tokenId: arg }));
    } else if (command === "all") {
      const result = await provision(admin, optionsFromEnv());
      console.log(`Published ${Object.entries(result.versions).map(([n, v]) => `${n}@v${v}`).join(", ")}. Next standup ${result.nextStandup}.`);
    } else {
      throw new Error("usage: npm run provision -- [all | app <name> | rotate | revoke <tokenId>]");
    }
  } finally {
    admin.close();
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) await main(process.argv.slice(2)).catch(error => { console.error(error.message); process.exit(1); });
