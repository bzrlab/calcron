import { MessageFlags, PermissionFlagsBits, SlashCommandBuilder } from "discord.js";
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
    const key = `temprole:${i.guildId}:${user.id}:${role.id}`;
    const sub = i.options.getSubcommand();
    if (sub === "remove") {
      await calcron.cancel(key, `${key}:remove:${i.id}`);
      await i.guild.members.removeRole({ user, role, reason: `Temporary role removed by ${i.user.tag}` });
      return i.reply({ content: `Removed ${role} from ${user}.`, flags: MessageFlags.Ephemeral, allowedMentions: { parse: [] } });
    }
    const duration = i.options.getString(sub === "add" ? "duration" : "by", true);
    if (!isDuration(duration)) throw new Error("Use Go duration syntax such as `30m`, `12h` or `168h`.");
    if (sub === "extend") {
      const { runAt } = await calcron.extend(key, duration, `${key}:extend:${i.id}`).catch(() => { throw new Error(`${user} has no temporary ${role}.`); });
      return i.reply({ content: `${role} for ${user} now expires ${relative(runAt)}.`, flags: MessageFlags.Ephemeral, allowedMentions: { parse: [] } });
    }
    const { runAt } = await calcron.set({
      key, event: "temprole.expire", after: duration, data: { guildId: i.guildId, userId: user.id, roleId: role.id },
      idempotencyKey: `${key}:add:${i.id}`,
    });
    await i.guild.members.addRole({ user, role, reason: `Temporary role granted by ${i.user.tag}` });
    await i.reply({ content: `Gave ${role} to ${user}; expires ${relative(runAt)}.`, flags: MessageFlags.Ephemeral, allowedMentions: { parse: [] } });
  },
  start(client) {
    onDelivery("temprole.expire", async ({ guildId, userId, roleId }) => {
      const member = await client.guilds.cache.get(guildId)?.members.fetch(userId).catch(() => null);
      if (member?.roles.cache.has(roleId)) await member.roles.remove(roleId, "Temporary role expired");
    });
  },
};
