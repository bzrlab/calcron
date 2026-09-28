import { randomInt } from "node:crypto";
import { EmbedBuilder, MessageFlags, PermissionFlagsBits, SlashCommandBuilder, type Message } from "discord.js";
import { calcron, isDuration, parseWhen, relative } from "../calcron.ts";
import { onDelivery, postOnce, textChannel, type Feature } from "../discord.ts";

const EMOJI = "🎉";
const key = (messageId: string) => `giveaway:${messageId}`;
const field = (message: Message, name: string) => message.embeds[0]?.fields.find(f => f.name === name)?.value;

export function pickWinners(entrants: string[], count: number) {
  const pool = [...new Set(entrants)];
  const n = Math.min(count, pool.length);
  for (let i = 0; i < n; i++) {
    const j = randomInt(i, pool.length);
    [pool[i], pool[j]] = [pool[j], pool[i]];
  }
  return pool.slice(0, n);
}

async function entrants(message: Message) {
  const reaction = message.reactions.resolve(EMOJI);
  const ids: string[] = [];
  for (let after: string | undefined; reaction;) {
    const page = await reaction.users.fetch({ limit: 100, after });
    ids.push(...page.filter(u => !u.bot).map(u => u.id));
    if (page.size < 100) break;
    after = page.lastKey();
  }
  return ids;
}

async function draw(message: Message, winners: number) {
  const picked = pickWinners(await entrants(message), winners);
  return picked.length ? picked.map(id => `<@${id}>`).join(" ") : "No valid entries.";
}

const restyle = (message: Message, description: string, color?: number) => {
  const embed = EmbedBuilder.from(message.embeds[0]).setDescription(description);
  return message.edit({ embeds: [color === undefined ? embed : embed.setColor(color)] });
};

export const giveaways: Feature = {
  id: "giveaway",
  commands: [
    new SlashCommandBuilder().setName("giveaway").setDescription("Run giveaways")
      .setDefaultMemberPermissions(PermissionFlagsBits.ManageGuild)
      .addSubcommand(s => s.setName("start").setDescription("Start a giveaway in this channel")
        .addStringOption(o => o.setName("prize").setDescription("Prize").setRequired(true).setMaxLength(200))
        .addStringOption(o => o.setName("ends").setDescription("24h, 90m, or 2026-10-01T20:00+06:00").setRequired(true))
        .addIntegerOption(o => o.setName("winners").setDescription("Number of winners").setMinValue(1).setMaxValue(20)))
      .addSubcommand(s => s.setName("extend").setDescription("Push the end time back")
        .addStringOption(o => o.setName("message").setDescription("Giveaway message ID").setRequired(true))
        .addStringOption(o => o.setName("by").setDescription("e.g. 1h").setRequired(true)))
      .addSubcommand(s => s.setName("end").setDescription("Draw winners now")
        .addStringOption(o => o.setName("message").setDescription("Giveaway message ID").setRequired(true)))
      .addSubcommand(s => s.setName("cancel").setDescription("Cancel without drawing")
        .addStringOption(o => o.setName("message").setDescription("Giveaway message ID").setRequired(true)))
      .addSubcommand(s => s.setName("reroll").setDescription("Draw new winners for an ended giveaway")
        .addStringOption(o => o.setName("message").setDescription("Giveaway message ID").setRequired(true)))
      .toJSON(),
  ],
  async command(i) {
    if (!i.inCachedGuild() || !i.channel) return;
    await i.deferReply({ flags: MessageFlags.Ephemeral });
    const sub = i.options.getSubcommand();
    if (sub === "start") {
      const prize = i.options.getString("prize", true);
      const winners = i.options.getInteger("winners") ?? 1;
      const deadline = parseWhen(i.options.getString("ends", true));
      const message = await i.channel.send({
        embeds: [new EmbedBuilder().setTitle(`${EMOJI} ${prize}`).setColor(0xeb459e).setDescription(`React with ${EMOJI} to enter.`)
          .addFields({ name: "Winners", value: String(winners), inline: true }, { name: "Host", value: `${i.user}`, inline: true })],
      });
      try {
        await message.react(EMOJI);
        const { runAt } = await calcron.set({
          key: key(message.id), event: "giveaway.end", ...deadline,
          data: { channelId: i.channelId, messageId: message.id, prize, winners },
          idempotencyKey: `${key(message.id)}:start`,
        });
        await restyle(message, `React with ${EMOJI} to enter.\nEnds ${relative(runAt)}`);
      } catch (error) {
        await message.delete().catch(() => {});
        throw error;
      }
      return i.editReply(`Giveaway started: ${message.url}`);
    }
    const message = await i.channel.messages.fetch(i.options.getString("message", true)).catch(() => null);
    if (!message || message.author.id !== i.client.user.id || !field(message, "Winners")) throw new Error("No giveaway with that message ID in this channel.");
    const ended = field(message, "Result");
    if (sub === "reroll") {
      if (!ended) throw new Error("That giveaway has not ended yet.");
      const result = await draw(message, Number(field(message, "Winners")));
      await message.reply({ content: `${EMOJI} Reroll: ${result}`, allowedMentions: { parse: ["users"] } });
      return i.editReply("Rerolled.");
    }
    if (ended) throw new Error("That giveaway already ended.");
    if (sub === "extend") {
      const by = i.options.getString("by", true);
      if (!isDuration(by)) throw new Error("Use Go duration syntax such as `30m` or `2h`.");
      const { runAt } = await calcron.extend(key(message.id), by, `${key(message.id)}:extend:${i.id}`);
      await restyle(message, `React with ${EMOJI} to enter.\nEnds ${relative(runAt)}`);
      return i.editReply(`Now ends ${relative(runAt)}.`);
    }
    if (sub === "cancel") {
      await calcron.cancel(key(message.id), `${key(message.id)}:cancel:${i.id}`);
      await restyle(message, "Cancelled.", 0x99aab5);
      return i.editReply("Giveaway cancelled.");
    }
    // Replacing the schedule with a zero delay draws now through the same durable delivery path.
    await calcron.set({
      key: key(message.id), event: "giveaway.end", after: "0s",
      data: { channelId: i.channelId, messageId: message.id, prize: message.embeds[0].title!.replace(`${EMOJI} `, ""), winners: Number(field(message, "Winners")) },
      idempotencyKey: `${key(message.id)}:end:${i.id}`,
    });
    await i.editReply("Drawing winners…");
  },
  start(client) {
    onDelivery("giveaway.end", async ({ channelId, messageId, prize, winners }, deliveryId) => {
      const channel = await textChannel(client, channelId);
      const message = await channel?.messages.fetch(messageId).catch(() => null);
      if (!channel || !message?.embeds[0]) return;
      let result = field(message, "Result");
      if (!result) {
        result = await draw(message, winners);
        await message.edit({ embeds: [EmbedBuilder.from(message.embeds[0]).setColor(0x99aab5).setDescription(`Ended ${relative(new Date().toISOString())}`).addFields({ name: "Result", value: result })] });
      }
      await postOnce(channel, deliveryId, new EmbedBuilder().setDescription(`${EMOJI} **${prize}** has ended. [Jump](${message.url})`), {
        content: result, allowedMentions: { parse: ["users"] },
      });
    });
  },
};
