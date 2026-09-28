import { EmbedBuilder, MessageFlags, SlashCommandBuilder } from "discord.js";
import { calcron, env } from "../calcron.ts";
import { onDelivery, postOnce, textChannel, type Feature } from "../discord.ts";

export const alerts: Feature = {
  id: "alert",
  commands: [
    new SlashCommandBuilder().setName("alert").setDescription("Page the moderators (once per cooldown)")
      .addStringOption(o => o.setName("reason").setDescription("What is happening?").setRequired(true).setMaxLength(300))
      .toJSON(),
  ],
  async command(i) {
    if (!i.inGuild()) return;
    const cooldown = env("ALERT_COOLDOWN", "15m");
    const { triggered } = await calcron.throttle({
      key: `alert:${i.guildId}`, event: "staff.alert", cooldown,
      data: { guildId: i.guildId, userId: i.user.id, channelId: i.channelId, reason: i.options.getString("reason", true) },
      idempotencyKey: `alert:${i.id}`,
    });
    await i.reply({
      content: triggered ? "🚨 Moderators have been paged." : `Moderators were paged in the last ${cooldown} and are on it.`,
      flags: MessageFlags.Ephemeral,
    });
  },
  start(client) {
    onDelivery("staff.alert", async ({ userId, channelId, reason }, deliveryId) => {
      const channel = await textChannel(client, env("MOD_CHANNEL_ID"));
      if (!channel) throw new Error("MOD_CHANNEL_ID is not a text channel");
      const role = env("MOD_ROLE_ID");
      await postOnce(channel, deliveryId, new EmbedBuilder().setTitle("🚨 Staff alert").setColor(0xed4245)
        .setDescription(reason).addFields({ name: "From", value: `<@${userId}>`, inline: true }, { name: "Where", value: `<#${channelId}>`, inline: true }),
      { content: `<@&${role}>`, allowedMentions: { roles: [role] } });
    });
  },
};
