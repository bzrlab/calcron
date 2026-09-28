import { PermissionFlagsBits, SlashCommandBuilder, MessageFlags } from "discord.js";
import { calcron, isDuration, relative } from "../calcron.ts";
import { onDelivery, type Feature } from "../discord.ts";

export const tempRoles: Feature = {
  id: "temprole",
  commands: [
    new SlashCommandBuilder().setName("temprole").setDescription("Roles that remove themselves")
      .setDefaultMemberPermissions(PermissionFlagsBits.ManageRoles)
      .addSubcommand(s => s.setName("add").setDescription("Give a role for a limited time")
        .addUserOption(o => o.setName("member").setDescription("Member").setRequired(true))
        .addRoleOption(o => o.setName("role").setDescription("Role").setRequired(true))
        .addStringOption(o => o.setName("duration").setDescription("30m, 12h, 168h").setRequired(true)))
      .addSubcommand(s => s.setName("extend").setDescription("Add time to a temporary role")
        .addUserOption(o => o.setName("member").setDescription("Member").setRequired(true))
        .addRoleOption(o => o.setName("role").setDescription("Role").setRequired(true))
        .addStringOption(o => o.setName("by").setDescription("30m, 12h, 168h").setRequired(true)))
      .addSubcommand(s => s.setName("remove").setDescription("Remove a temporary role now")
        .addUserOption(o => o.setName("member").setDescription("Member").setRequired(true))
        .addRoleOption(o => o.setName("role").setDescription("Role").setRequired(true)))
      .toJSON(),
  ],
  async command(i) {
    if (!i.inCachedGuild()) return;
    const user = i.options.getUser("member", true);
    const role = i.options.getRole("role", true);
    if (role.managed || (i.guild.ownerId !== i.user.id && role.position >= i.member.roles.highest.position)) {
      throw new Error("You can only manage roles below your highest role.");
    }
    await i.deferReply({ flags: MessageFlags.Ephemeral });
    const key = `temprole:${i.guildId}:${user.id}:${role.id}`;
    const sub = i.options.getSubcommand();
    if (sub === "remove") {
      await calcron.cancel(key, `${key}:remove:${i.id}`);
      await i.guild.members.removeRole({ user, role, reason: `Temporary role removed by ${i.user.tag}` });
      return i.editReply({ content: `Removed ${role} from ${user}.`, allowedMentions: { parse: [] } });
    }
    const duration = i.options.getString(sub === "add" ? "duration" : "by", true);
    if (!isDuration(duration)) throw new Error("Use Go duration syntax such as `30m`, `12h` or `168h`.");
    if (sub === "extend") {
      const { runAt } = await calcron.extend(key, duration, `${key}:extend:${i.id}`).catch((error: Error) => {
        throw error.message === "schedule not found" ? new Error(`${user} has no temporary ${role}.`) : error;
      });
      return i.editReply({ content: `${role} for ${user} now expires ${relative(runAt)}.`, allowedMentions: { parse: [] } });
    }
    const { runAt } = await calcron.set({
      key, event: "temprole.expire", after: duration, data: { guildId: i.guildId, userId: user.id, roleId: role.id },
      idempotencyKey: `${key}:add:${i.id}`,
    });
    await i.guild.members.addRole({ user, role, reason: `Temporary role granted by ${i.user.tag}` });
    await i.editReply({ content: `Gave ${role} to ${user}; expires ${relative(runAt)}.`, allowedMentions: { parse: [] } });
  },
  start(client) {
    onDelivery("temprole.expire", async ({ guildId, userId, roleId }) => {
      const member = await client.guilds.cache.get(guildId)?.members.fetch(userId).catch(() => null);
      if (member?.roles.cache.has(roleId)) await member.roles.remove(roleId, "Temporary role expired");
    });
  },
};
