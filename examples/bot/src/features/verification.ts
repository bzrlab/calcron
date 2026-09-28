import { ButtonStyle, EmbedBuilder, MessageFlags, PermissionFlagsBits, type ButtonInteraction, type GuildMember, type PartialGuildMember } from "discord.js";
import { calcron, env, verifyKey, type Join, type Verdict } from "../calcron.ts";
import { button, onDelivery, postOnce, row, textChannel, type Feature } from "../discord.ts";

const timeout = () => env("VERIFY_TIMEOUT", "24h");
const joinOf = (m: GuildMember | PartialGuildMember): Join | null =>
  m.user.bot || !m.joinedTimestamp ? null : { guildId: m.guild.id, userId: m.id, joinedAt: String(m.joinedTimestamp) };

async function decide(i: ButtonInteraction, verdict: Verdict) {
  await i.deferUpdate();
  const { matched } = await calcron.signal("member.verified", verifyKey(verdict), `${verifyKey(verdict)}:verdict:${i.id}`, verdict);
  if (!matched) return i.followUp({ content: "No verification is waiting for that member. It may already be decided.", flags: MessageFlags.Ephemeral });
  await calcron.cancel(`${verifyKey(verdict)}:timeout`, `${verifyKey(verdict)}:timeout-cancel:${i.id}`);
  await i.editReply({ components: [], embeds: [EmbedBuilder.from(i.message.embeds[0]).setColor(verdict.approved ? 0x57f287 : 0xed4245).setDescription(verdict.reason)] });
}

export const verification: Feature = {
  id: "verify",
  commands: [],
  async button(i, [action, userId, joinedAt]) {
    if (!i.inGuild()) return;
    const join = { guildId: i.guildId, userId, joinedAt };
    if (action === "accept") {
      if (i.user.id !== userId) return i.reply({ content: "Only the new member can accept.", flags: MessageFlags.Ephemeral });
      return decide(i, { ...join, approved: true, by: i.user.id, reason: `✅ ${i.user} accepted the rules.` });
    }
    if (!i.memberPermissions.has(PermissionFlagsBits.KickMembers)) return i.reply({ content: "Moderators only.", flags: MessageFlags.Ephemeral });
    return decide(i, { ...join, approved: action === "approve", by: i.user.id, reason: `${action === "approve" ? "✅ Approved" : "⛔ Rejected"} by ${i.user}.` });
  },
  start(client) {
    client.on("guildMemberAdd", async m => {
      const join = joinOf(m);
      if (!join) return;
      try {
        // Timeout first: if start then fails, the timeout's signal simply matches nothing.
        await calcron.set({ key: `${verifyKey(join)}:timeout`, event: "verify.timeout", after: timeout(), data: join, idempotencyKey: `${verifyKey(join)}:timeout` });
        await calcron.start("member-verification", `${verifyKey(join)}:start`, join);
      } catch (error) {
        console.error("[verify] could not start verification", error);
      }
    });
    client.on("guildMemberRemove", async m => {
      const join = joinOf(m);
      if (!join) return;
      try {
        await calcron.cancel(`${verifyKey(join)}:timeout`, `${verifyKey(join)}:left:cancel`);
        await calcron.signal("member.verified", verifyKey(join), `${verifyKey(join)}:left`, { ...join, approved: false, by: "discord", reason: "Left before verifying." });
      } catch (error) {
        console.error("[verify] could not close verification", error);
      }
    });
    onDelivery("verify.prompt", async ({ userId, joinedAt }, deliveryId) => {
      const channel = await textChannel(client, env("VERIFY_CHANNEL_ID"));
      if (!channel) throw new Error("VERIFY_CHANNEL_ID is not a text channel");
      await postOnce(channel, deliveryId, new EmbedBuilder().setTitle("👋 Welcome!").setColor(0x5865f2)
        .setDescription(`Read the rules, then press **I accept** within ${timeout()} to unlock the server.`), {
        content: `<@${userId}>`, allowedMentions: { users: [userId] },
        components: [row(
          button(`verify:accept:${userId}:${joinedAt}`, "I accept", ButtonStyle.Success),
          button(`verify:approve:${userId}:${joinedAt}`, "Approve (mod)"),
          button(`verify:reject:${userId}:${joinedAt}`, "Reject (mod)", ButtonStyle.Danger),
        )],
      });
    });
    // The timeout is a direct schedule; it reports the outcome to the workflow like any other signal.
    onDelivery("verify.timeout", async (join, deliveryId) => {
      await calcron.signal("member.verified", verifyKey(join), `${verifyKey(join)}:timeout:${deliveryId}`, {
        ...join, approved: false, by: "calcron", reason: `⌛ Did not verify within ${timeout()}.`,
      });
    });
    onDelivery("verify.grant", async ({ guildId, userId }) => {
      const member = await client.guilds.cache.get(guildId)?.members.fetch(userId).catch(() => null);
      await member?.roles.add(env("VERIFIED_ROLE_ID"), "Verification approved");
    });
    onDelivery("verify.reject", async ({ guildId, userId, reason }) => {
      const member = await client.guilds.cache.get(guildId)?.members.fetch(userId).catch(() => null);
      if (member && !member.roles.cache.has(env("VERIFIED_ROLE_ID"))) await member.kick(reason.replace(/<@!?\d+>/g, "").trim());
    });
  },
};
