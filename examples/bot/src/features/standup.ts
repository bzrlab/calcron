import { ChannelType, EmbedBuilder, MessageFlags, PermissionFlagsBits, SlashCommandBuilder, ThreadAutoArchiveDuration, type Client } from "discord.js";
import { calcron, env } from "../calcron.ts";
import { onDelivery, type Feature } from "../discord.ts";

const PREFIX = "Standup · ";

async function standupChannel(client: Client<true>, id: string) {
  const channel = await client.channels.fetch(id).catch(() => null);
  if (channel?.type !== ChannelType.GuildText) throw new Error(`Standup channel ${id} is not a text channel`);
  return channel;
}

export const standup: Feature = {
  id: "standup",
  commands: [
    new SlashCommandBuilder().setName("standup").setDescription("Start an ad-hoc standup in this channel")
      .setDefaultMemberPermissions(PermissionFlagsBits.ManageGuild)
      .toJSON(),
  ],
  async command(i) {
    if (i.channel?.type !== ChannelType.GuildText) throw new Error("Run this in a text channel.");
    const { instanceId } = await calcron.start("daily-standup", `standup:manual:${i.id}`, { channelId: i.channelId });
    await i.reply({ content: `Standup workflow started (\`${instanceId}\`).`, flags: MessageFlags.Ephemeral });
  },
  start(client) {
    onDelivery("standup.open", async ({ channelId }) => {
      const channel = await standupChannel(client, channelId);
      const name = PREFIX + new Date().toLocaleDateString("en-CA", { timeZone: env("CALENDAR_TIMEZONE", "Asia/Dhaka") });
      const { threads } = await channel.threads.fetchActive();
      if (threads.some(t => t.parentId === channel.id && t.name === name)) return;
      const post = await channel.send({ embeds: [new EmbedBuilder().setTitle("🗓️ Daily standup").setColor(0x5865f2)
        .setDescription("Reply in the thread with **Yesterday**, **Today** and **Blockers**. It closes automatically.")] });
      await post.startThread({ name, autoArchiveDuration: ThreadAutoArchiveDuration.OneDay });
    });
    onDelivery("standup.close", async ({ channelId }) => {
      const channel = await standupChannel(client, channelId);
      const { threads } = await channel.threads.fetchActive();
      for (const thread of threads.filter(t => t.parentId === channel.id && t.ownerId === client.user.id && t.name.startsWith(PREFIX)).values()) {
        const authors = [...new Set((await thread.messages.fetch({ limit: 100 })).filter(m => !m.author.bot).map(m => m.author.id))];
        await thread.send({ embeds: [new EmbedBuilder().setTitle("Standup closed").setColor(0x99aab5)
          .setDescription(authors.length ? `${authors.length} update(s) from ${authors.map(id => `<@${id}>`).join(", ")}.` : "No updates today.")] });
        await thread.setArchived(true, "Standup window ended");
      }
    });
  },
};
