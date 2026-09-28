import { Client, Events, GatewayIntentBits, InteractionContextType, MessageFlags } from "discord.js";
import { calcron, env } from "./calcron.ts";
import type { Feature } from "./discord.ts";
import { alerts } from "./features/alerts.ts";
import { giveaways } from "./features/giveaways.ts";
import { ops } from "./features/ops.ts";
import { reminders } from "./features/reminders.ts";
import { standup } from "./features/standup.ts";
import { tempRoles } from "./features/temproles.ts";
import { tickets } from "./features/tickets.ts";
import { verification } from "./features/verification.ts";

const features: Feature[] = [reminders, tickets, tempRoles, alerts, giveaways, verification, standup, ops];
env("CALCRON_TOKEN");

const client = new Client({ intents: [GatewayIntentBits.Guilds, GatewayIntentBits.GuildMembers, GatewayIntentBits.GuildMessages] });

client.once(Events.ClientReady, async ready => {
  // Calcron may push queued deliveries right after auth, so handlers and the Discord cache must be ready first.
  for (const feature of features) feature.start?.(ready);
  const commands = features.flatMap(f => f.commands).map(c => ({ ...c, contexts: [InteractionContextType.Guild] }));
  const guildId = process.env.DISCORD_GUILD_ID;
  try {
    if (guildId) {
      const guild = ready.guilds.cache.get(guildId);
      if (!guild) throw new Error(`Bot is not in DISCORD_GUILD_ID guild ${guildId}`);
      await guild.commands.set(commands);
    } else await ready.application.commands.set(commands);
    await calcron.connect();
  } catch (error) {
    console.error("Startup failed:", error instanceof Error ? error.message : error);
    await client.destroy();
    process.exit(1);
  }
  console.log(`${ready.user.tag} ready: ${commands.length} commands, Calcron connected`);
});

client.on(Events.InteractionCreate, async i => {
  try {
    if (i.isChatInputCommand()) await features.find(f => f.commands.some(c => c.name === i.commandName))?.command?.(i);
    else if (i.isButton()) {
      const [id, ...args] = i.customId.split(":");
      await features.find(f => f.id === id)?.button?.(i, args);
    }
  } catch (error) {
    if (!i.isRepliable()) return;
    const content = `⚠️ ${error instanceof Error ? error.message : String(error)}`;
    await (i.deferred || i.replied ? i.followUp({ content, flags: MessageFlags.Ephemeral }) : i.reply({ content, flags: MessageFlags.Ephemeral })).catch(() => {});
  }
});

for (const signal of ["SIGINT", "SIGTERM"] as const) process.once(signal, () => { calcron.close(); void client.destroy().then(() => process.exit(0)); });

await client.login(env("DISCORD_TOKEN"));
