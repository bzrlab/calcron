import {
  ActionRowBuilder, ButtonBuilder, ButtonStyle, type ButtonInteraction, type ChatInputCommandInteraction,
  type Client, type EmbedBuilder, type GuildTextBasedChannel, type MessageCreateOptions, type RESTPostAPIChatInputApplicationCommandsJSONBody,
} from "discord.js";
import type { EventName } from "@bzrlab/calcron";
import { calcron, type Events } from "./calcron.ts";

export type Feature = {
  /** Button custom IDs start with `${id}:`. */
  id: string;
  commands: RESTPostAPIChatInputApplicationCommandsJSONBody[];
  command?: (i: ChatInputCommandInteraction) => Promise<unknown>;
  button?: (i: ButtonInteraction, args: string[]) => Promise<unknown>;
  start?: (client: Client<true>) => void;
};

/** Acknowledges only after the handler's effect succeeds. A throw leaves the delivery for Calcron to retry, then block after five attempts. */
export function onDelivery<Name extends EventName<Events>>(event: Name, handler: (data: Events[Name], deliveryId: string) => Promise<void>) {
  calcron.on(event, async delivery => {
    try {
      await handler(delivery.data, delivery.id);
      await delivery.ack();
    } catch (error) {
      console.error(`[calcron] ${event} ${delivery.id} failed, will retry:`, error);
    }
  });
}

export async function textChannel(client: Client<true>, id: string) {
  const channel = await client.channels.fetch(id).catch(() => null);
  return channel?.isTextBased() && !channel.isDMBased() ? channel : null;
}

/** Deliveries are at least once: stamp the delivery ID on the post and skip it if a redelivery finds it already sent. */
export async function postOnce(channel: GuildTextBasedChannel, deliveryId: string, embed: EmbedBuilder, extra: Omit<MessageCreateOptions, "embeds"> = {}) {
  const footer = `delivery ${deliveryId}`;
  const recent = await channel.messages.fetch({ limit: 50 });
  const sent = recent.find(m => m.author.id === channel.client.user.id && m.embeds.some(e => e.footer?.text === footer));
  return sent ?? channel.send({ ...extra, embeds: [embed.setFooter({ text: footer })] });
}

export const button = (customId: string, label: string, style = ButtonStyle.Secondary) => new ButtonBuilder().setCustomId(customId).setLabel(label).setStyle(style);
export const row = (...buttons: ButtonBuilder[]) => new ActionRowBuilder<ButtonBuilder>().addComponents(buttons);
