import { ButtonStyle, EmbedBuilder, MessageFlags, PermissionFlagsBits, type ButtonInteraction } from "discord.js";
import { calcron, env, verifyKey, type Verdict } from "../calcron.ts";
import { button, onDelivery, postOnce, row, textChannel, type Feature } from "../discord.ts";

async function decide(i: ButtonInteraction, verdict: Verdict) {
  const { matched } = await calcron.signal("member.verified", verifyKey(verdict), `${verifyKey(verdict)}:verdict:${i.id}`, verdict);
  if (!matched) return i.reply({ content: "No verification is waiting for that member. It may already be decided.", flags: MessageFlags.Ephemeral });
  await calcron.cancel(`${verifyKey(verdict)}:timeout`, `${verifyKey(verdict)}:timeout-cancel:${i.id}`);
  await i.update({ components: [], embeds: [EmbedBuilder.from(i.message.embeds[0]).setColor(verdict.approved ? 0x57f287 : 0xed4245).setDescription(verdict.reason)] });
}

export const verification: Feature = {
  id: "verify",
  commands: [],
  async button(i, [action, userId]) {
    if (!i.inGuild()) return;
    const member = { guildId: i.guildId, userId };
    if (action === "accept") {
      if (i.user.id !== userId) return i.reply({ content: "Only the new member can accept.", flags: MessageFlags.Ephemeral });
      return decide(i, { ...member, approved: true, by: i.user.id, reason: `✅ ${i.user} accepted the rules.` });
    }
    if (!i.memberPermissions.has(PermissionFlagsBits.KickMembers)) return i.reply({ content: "Moderators only.", flags: MessageFlags.Ephemeral });
    return decide(i, { ...member, approved: action === "approve", by: i.user.id, reason: `${action === "approve" ? "✅ Approved" : "⛔ Rejected"} by ${i.user}.` });
  },
  start(client) {
    client.on("guildMemberAdd", async m => {
      if (m.user.bot) return;
      const member = { guildId: m.guild.id, userId: m.id };
      const join = `${verifyKey(member)}:${m.joinedTimestamp}`;
      try {
        await calcron.start("member-verification", `${join}:start`, member);
        await calcron.set({ key: `${verifyKey(member)}:timeout`, event: "verify.timeout", after: env("VERIFY_TIMEOUT", "24h"), data: member, idempotencyKey: `${join}:timeout` });
      } catch (error) {
        console.error("[verify] could not start verification", error);
      }
    });
    onDelivery("verify.prompt", async ({ userId }, deliveryId) => {
      const channel = await textChannel(client, env("VERIFY_CHANNEL_ID"));
      if (!channel) throw new Error("VERIFY_CHANNEL_ID is not a text channel");
      await postOnce(channel, deliveryId, new EmbedBuilder().setTitle("👋 Welcome!").setColor(0x5865f2)
        .setDescription(`Read the rules, then press **I accept** within ${env("VERIFY_TIMEOUT", "24h")} to unlock the server.`), {
        content: `<@${userId}>`, allowedMentions: { users: [userId] },
        components: [row(
          button(`verify:accept:${userId}`, "I accept", ButtonStyle.Success),
          button(`verify:approve:${userId}`, "Approve (mod)"),
          button(`verify:reject:${userId}`, "Reject (mod)", ButtonStyle.Danger),
        )],
      });
    });
    // The timeout is a direct schedule; it reports the outcome to the workflow like any other signal.
    onDelivery("verify.timeout", async (member, deliveryId) => {
      await calcron.signal("member.verified", verifyKey(member), `${verifyKey(member)}:timeout:${deliveryId}`, {
        ...member, approved: false, by: "calcron", reason: `⌛ Did not verify within ${env("VERIFY_TIMEOUT", "24h")}.`,
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
