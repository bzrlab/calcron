import { ButtonStyle, EmbedBuilder, MessageFlags, SlashCommandBuilder } from "discord.js";
import { calcron, parseWhen, relative } from "../calcron.ts";
import { button, onDelivery, postOnce, row, textChannel, type Feature } from "../discord.ts";

const key = (userId: string, reminderId: string) => `remind:${userId}:${reminderId}`;
const snoozeRow = (userId: string, reminderId: string) => row(
  button(`remind:snooze:${userId}:${reminderId}:10m`, "Snooze 10m"),
  button(`remind:snooze:${userId}:${reminderId}:1h`, "Snooze 1h"),
  button(`remind:snooze:${userId}:${reminderId}:24h`, "Tomorrow"),
);

export const reminders: Feature = {
  id: "remind",
  commands: [
    new SlashCommandBuilder().setName("remind").setDescription("Set a durable reminder")
      .addStringOption(o => o.setName("when").setDescription("10m, 2h30m, or 2026-10-01T09:00+06:00").setRequired(true))
      .addStringOption(o => o.setName("text").setDescription("What to remind you about").setRequired(true).setMaxLength(500))
      .toJSON(),
  ],
  async command(i) {
    const deadline = parseWhen(i.options.getString("when", true));
    const reminderId = i.id;
    const { runAt } = await calcron.set({
      key: key(i.user.id, reminderId), event: "reminder.due", ...deadline,
      data: { reminderId, userId: i.user.id, channelId: i.channelId, text: i.options.getString("text", true) },
      idempotencyKey: `remind:${reminderId}:set`,
    });
    await i.reply({
      content: `⏰ Reminder set for ${relative(runAt)}.`,
      components: [row(button(`remind:cancel:${i.user.id}:${reminderId}`, "Cancel reminder", ButtonStyle.Danger))],
      flags: MessageFlags.Ephemeral,
    });
  },
  async button(i, [action, userId, reminderId, by]) {
    if (i.user.id !== userId) return i.reply({ content: "That reminder belongs to someone else.", flags: MessageFlags.Ephemeral });
    if (action === "cancel") {
      await calcron.cancel(key(userId, reminderId), `remind:${reminderId}:cancel:${i.id}`);
      return i.update({ content: "Reminder cancelled.", components: [] });
    }
    // Extending a delivered schedule reschedules it from now, which is exactly a snooze.
    const { runAt } = await calcron.extend(key(userId, reminderId), by, `remind:${reminderId}:snooze:${i.id}`);
    await i.update({ components: [] });
    await i.followUp({ content: `💤 Snoozed until ${relative(runAt)}.`, flags: MessageFlags.Ephemeral });
  },
  start(client) {
    onDelivery("reminder.due", async ({ reminderId, userId, channelId, text }, deliveryId) => {
      const channel = await textChannel(client, channelId);
      if (!channel) return;
      await postOnce(channel, deliveryId, new EmbedBuilder().setTitle("⏰ Reminder").setDescription(text).setColor(0x5865f2), {
        content: `<@${userId}>`, allowedMentions: { users: [userId] }, components: [snoozeRow(userId, reminderId)],
      });
    });
  },
};
