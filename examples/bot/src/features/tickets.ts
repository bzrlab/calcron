import { ButtonStyle, ChannelType, EmbedBuilder, MessageFlags, SlashCommandBuilder, type AnyThreadChannel, type Channel, type Client } from "discord.js";
import { calcron, env } from "../calcron.ts";
import { button, onDelivery, postOnce, row, type Feature } from "../discord.ts";

const idle = () => env("TICKET_IDLE", "24h");
const grace = () => env("TICKET_GRACE", "1h");
const isTicket = (channel: Channel | null, client: Client<true>): channel is AnyThreadChannel =>
  channel?.type === ChannelType.PrivateThread && channel.ownerId === client.user.id;

// ponytail: per-process throttle; a restart just costs one extra touch per thread.
const touched = new Map<string, number>();

/** Any human message replaces the idle deadline and cancels a pending close, at most once a minute per thread. */
async function touch(threadId: string, intent: string) {
  if (Date.now() - (touched.get(threadId) ?? 0) < 60_000) return;
  touched.set(threadId, Date.now());
  await calcron.set({
    key: `ticket:${threadId}:idle`, event: "ticket.idle", after: idle(), data: { threadId },
    chain: { key: `ticket:${threadId}:close`, event: "ticket.close", after: grace(), data: { threadId } },
    idempotencyKey: `ticket:${threadId}:touch:${intent}`,
  });
  await calcron.cancel(`ticket:${threadId}:close`, `ticket:${threadId}:reprieve:${intent}`);
}

async function close(thread: AnyThreadChannel, reason: string, intent: string) {
  touched.delete(thread.id);
  await calcron.cancel(`ticket:${thread.id}:idle`, `ticket:${thread.id}:close-idle:${intent}`);
  await calcron.cancel(`ticket:${thread.id}:close`, `ticket:${thread.id}:close-close:${intent}`);
  if (thread.archived) return;
  await thread.send({ embeds: [new EmbedBuilder().setDescription(`🔒 Ticket closed: ${reason}`).setColor(0x99aab5)] });
  await thread.setLocked(true);
  await thread.setArchived(true, reason);
}

export const tickets: Feature = {
  id: "ticket",
  commands: [
    new SlashCommandBuilder().setName("ticket").setDescription("Private support tickets")
      .addSubcommand(s => s.setName("open").setDescription("Open a private ticket with staff")
        .addStringOption(o => o.setName("topic").setDescription("What do you need help with?").setRequired(true).setMaxLength(200)))
      .addSubcommand(s => s.setName("close").setDescription("Close this ticket"))
      .toJSON(),
  ],
  async command(i) {
    if (i.options.getSubcommand() === "close") {
      if (!isTicket(i.channel, i.client)) throw new Error("Run this inside a ticket thread.");
      await i.reply({ content: "Closing…", flags: MessageFlags.Ephemeral });
      return close(i.channel, `closed by ${i.user}`, i.id);
    }
    if (i.channel?.type !== ChannelType.GuildText) throw new Error("Open tickets from a regular text channel.");
    await i.deferReply({ flags: MessageFlags.Ephemeral });
    const modRole = env("MOD_ROLE_ID");
    const topic = i.options.getString("topic", true);
    const thread = await i.channel.threads.create({ name: `ticket-${i.user.username}`.slice(0, 100), type: ChannelType.PrivateThread, invitable: false });
    await thread.members.add(i.user.id);
    await thread.send({
      content: `${i.user} <@&${modRole}>`,
      allowedMentions: { users: [i.user.id], roles: [modRole] },
      embeds: [new EmbedBuilder().setTitle("🎫 New ticket").setDescription(topic).setColor(0x57f287)
        .setFooter({ text: `Closes after ${idle()} without replies, with a ${grace()} warning.` })],
      components: [row(button("ticket:close", "Close ticket", ButtonStyle.Danger))],
    });
    await touch(thread.id, `open:${i.id}`);
    await i.editReply(`Ticket opened: ${thread}`);
  },
  async button(i) {
    if (!isTicket(i.channel, i.client)) return;
    await i.reply({ content: "Closing…", flags: MessageFlags.Ephemeral });
    await close(i.channel, `closed by ${i.user}`, i.id);
  },
  start(client) {
    client.on("messageCreate", message => {
      if (message.author.bot || !isTicket(message.channel, client)) return;
      touch(message.channelId, message.id).catch(error => console.error("[ticket] touch failed", error));
    });
    onDelivery("ticket.idle", async ({ threadId }, deliveryId) => {
      const thread = await client.channels.fetch(threadId).catch(() => null);
      if (!isTicket(thread, client) || thread.archived) return;
      await postOnce(thread, deliveryId, new EmbedBuilder().setColor(0xfee75c)
        .setDescription(`⏳ No replies for ${idle()}. This ticket closes in ${grace()} unless someone responds.`));
    });
    onDelivery("ticket.close", async ({ threadId }, deliveryId) => {
      const thread = await client.channels.fetch(threadId).catch(() => null);
      if (!isTicket(thread, client) || thread.archived) return;
      // A human reply that raced the warning's acknowledgement wins over the chained close.
      const [last] = (await thread.messages.fetch({ limit: 1 })).values();
      if (last && !last.author.bot) return;
      await close(thread, `no activity for ${idle()}`, deliveryId);
    });
  },
};
