import { ButtonStyle, EmbedBuilder, MessageFlags, PermissionFlagsBits, SlashCommandBuilder } from "discord.js";
import { env, withAdmin, type Admin } from "../calcron.ts";
import { CALENDAR, loadCalendar } from "../provision.ts";
import { button, row, type Feature } from "../discord.ts";

type Stats = { scheduled: number; pending: number; blocked: number; workflows: number };
type DeliveryRow = { id: string; event: string; status: string; attempts: number; created_at: string };
type StartRow = { name: string; workflow_name: string; calendar_name: string; local_time: string; missed_policy: string; input: unknown; next_at: string };

const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const ts = (iso: string, style = "F") => `<t:${Math.floor(new Date(iso).getTime() / 1000)}:${style}>`;

const startSchedules = (admin: Admin, appId: string) => admin.call<StartRow[]>("dashboard.list", { name: "start_schedules", applicationId: appId });

/** Overrides one date, then re-sets start schedules: a calendar edit alone leaves an already-computed next occurrence in place. */
export async function overrideDate(admin: Admin, appId: string, date: string, status: "off" | "on" | "default") {
  const calendar = await loadCalendar(admin, appId);
  if (!calendar) throw new Error("No business calendar yet. Run `npm run provision`.");
  const overrides = { ...calendar.overrides };
  if (status === "default") delete overrides[date];
  else overrides[date] = status === "on";
  await admin.call("calendar.set", { applicationId: appId, name: CALENDAR, data: { ...calendar, overrides } });
  const next: Record<string, string> = {};
  for (const s of (await startSchedules(admin, appId)).filter(s => s.calendar_name === CALENDAR)) {
    next[s.name] = (await admin.call<{ nextAt: string }>("start-schedule.set", {
      applicationId: appId, name: s.name, workflow: s.workflow_name, calendar: s.calendar_name,
      localTime: s.local_time, missedPolicy: s.missed_policy, data: s.input,
    })).nextAt;
  }
  return next;
}

export const ops: Feature = {
  id: "calcron",
  commands: [
    new SlashCommandBuilder().setName("calcron").setDescription("Operate the Calcron scheduler behind this bot")
      .setDefaultMemberPermissions(PermissionFlagsBits.Administrator)
      .addSubcommand(s => s.setName("stats").setDescription("Scheduler health"))
      .addSubcommand(s => s.setName("blocked").setDescription("Deliveries that failed five times; replay or cancel them"))
      .addSubcommand(s => s.setName("calendar").setDescription("Business calendar and upcoming standups"))
      .addSubcommand(s => s.setName("holiday").setDescription("Override one calendar date")
        .addStringOption(o => o.setName("date").setDescription("YYYY-MM-DD").setRequired(true))
        .addStringOption(o => o.setName("status").setDescription("Override").setRequired(true).addChoices(
          { name: "Holiday (no standup)", value: "off" },
          { name: "Extra working day", value: "on" },
          { name: "Back to weekly default", value: "default" },
        )))
      .toJSON(),
  ],
  async command(i) {
    await i.deferReply({ flags: MessageFlags.Ephemeral });
    const appId = env("CALCRON_APP_ID");
    const sub = i.options.getSubcommand();

    if (sub === "stats") {
      const stats = await withAdmin(a => a.call<Stats>("dashboard.stats"));
      return i.editReply({ embeds: [new EmbedBuilder().setTitle("Calcron (all applications)").setColor(stats.blocked ? 0xed4245 : 0x57f287).addFields(
        { name: "Scheduled", value: String(stats.scheduled), inline: true },
        { name: "Pending deliveries", value: String(stats.pending), inline: true },
        { name: "Blocked deliveries", value: String(stats.blocked), inline: true },
        { name: "Active workflows", value: String(stats.workflows), inline: true },
      )] });
    }

    if (sub === "blocked") {
      const blocked = (await withAdmin(a => a.call<DeliveryRow[]>("dashboard.list", { name: "deliveries", applicationId: appId })))
        .filter(d => d.status === "blocked").slice(0, 5);
      if (!blocked.length) return i.editReply("No blocked deliveries. 🎉");
      return i.editReply({
        embeds: [new EmbedBuilder().setTitle("Blocked deliveries").setColor(0xed4245)
          .setDescription(blocked.map((d, n) => `**${n + 1}.** \`${d.event}\` · ${d.attempts} attempts · ${ts(d.created_at, "R")}\n\`${d.id}\``).join("\n"))],
        components: blocked.map((d, n) => row(
          button(`calcron:replay:${d.id}`, `Replay #${n + 1}`, ButtonStyle.Primary),
          button(`calcron:cancel:${d.id}`, `Cancel #${n + 1}`, ButtonStyle.Danger),
        )),
      });
    }

    if (sub === "calendar") {
      const embed = await withAdmin(async a => {
        const calendar = await loadCalendar(a, appId);
        if (!calendar) throw new Error("No business calendar yet. Run `npm run provision`.");
        const daily = (await startSchedules(a, appId)).find(s => s.name === "daily-standup");
        const localTime = daily?.local_time ?? env("STANDUP_TIME", "10:00");
        const now = new Date();
        const [{ nextAt }, { occurrences }] = await Promise.all([
          a.call<{ nextAt: string }>("calendar.next", { applicationId: appId, calendar: CALENDAR, localTime }),
          a.call<{ occurrences: string[] }>("calendar.occurrences", {
            applicationId: appId, calendar: CALENDAR, localTime,
            at: now.toISOString(), until: new Date(now.getTime() + 14 * 86_400_000).toISOString(),
          }),
        ]);
        const overrides = Object.entries(calendar.overrides ?? {}).sort();
        return new EmbedBuilder().setTitle(`📅 ${CALENDAR}`).setColor(0x5865f2).addFields(
          { name: "Timezone", value: calendar.timezone, inline: true },
          { name: "Working days", value: calendar.weekdays.map(d => DAYS[d]).join(", "), inline: true },
          { name: "Missed standups", value: daily?.missed_policy ?? "not provisioned", inline: true },
          { name: "Next standup", value: `${ts(nextAt)} (${ts(nextAt, "R")})` },
          { name: "Next 14 days", value: occurrences.map(o => ts(o, "D")).join("\n") || "none" },
          { name: "Overrides", value: overrides.map(([d, on]) => `${d}: ${on ? "extra working day" : "holiday"}`).join("\n") || "none" },
        );
      });
      return i.editReply({ embeds: [embed] });
    }

    const date = i.options.getString("date", true);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || Number.isNaN(Date.parse(date))) throw new Error("Use a date like `2026-12-16`.");
    const status = i.options.getString("status", true) as "off" | "on" | "default";
    const next = await withAdmin(a => overrideDate(a, appId, date, status));
    const label = status === "off" ? "a holiday" : status === "on" ? "an extra working day" : "back to the weekly default";
    await i.editReply([`**${date}** is now ${label}.`, ...Object.entries(next).map(([name, at]) => `**${name}** next ${ts(at)}`)].join("\n"));
  },
  async button(i, [action, deliveryId]) {
    if (!i.memberPermissions?.has(PermissionFlagsBits.Administrator)) return i.reply({ content: "Administrators only.", flags: MessageFlags.Ephemeral });
    const result = await withAdmin(a => a.call<{ replayed?: boolean; cancelled?: boolean }>(action === "replay" ? "delivery.replay" : "delivery.cancel", { deliveryId }));
    const done = action === "replay" ? result.replayed : result.cancelled;
    await i.reply({ content: done ? `Delivery \`${deliveryId}\` ${action === "replay" ? "queued for replay" : "cancelled"}.` : "Nothing changed; it was already handled.", flags: MessageFlags.Ephemeral });
  },
};
