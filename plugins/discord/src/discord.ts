import { Client, Events, GatewayIntentBits, Partials, type Message } from "discord.js";
import { ConfigError, type Attachment, type ChannelContext, type ChannelDefinition, type SendRequest } from "@theopenbee/channel-sdk";

import { attachmentType, chunkText, decodeRaw, encodeRaw, planAttachments, stripMention } from "./helpers.js";

const DEFAULT_MAX_MEDIA_SIZE = 10 * 1024 * 1024;

interface DiscordConfig {
  token: string;
  allowedGuilds: string[];
  maxMediaSize: number;
}

function readConfig(raw: Record<string, unknown>): DiscordConfig {
  const token = typeof raw.token === "string" ? raw.token.trim() : "";
  if (!token) throw new ConfigError("token is required");
  const allowedGuilds = Array.isArray(raw.allowed_guilds) ? raw.allowed_guilds.filter((g): g is string => typeof g === "string") : [];
  const maxMediaSize = typeof raw.max_media_size === "number" && raw.max_media_size > 0 ? raw.max_media_size : DEFAULT_MAX_MEDIA_SIZE;
  return { token, allowedGuilds, maxMediaSize };
}

function errorCode(err: unknown): string | undefined {
  return typeof err === "object" && err !== null && "code" in err ? String((err as { code: unknown }).code) : undefined;
}

function extension(name: string): string {
  const dot = name.lastIndexOf(".");
  return dot > 0 ? name.slice(dot) : "";
}

export function createDiscordChannel(): ChannelDefinition {
  let client: Client | undefined;
  let cfg: DiscordConfig;

  const downloadAttachments = async (ctx: ChannelContext, message: Message): Promise<{ attachments: Attachment[]; notes: string[] }> => {
    const list = [...message.attachments.values()].map((a) => ({ name: a.name, size: a.size, url: a.url, contentType: a.contentType }));
    const plan = planAttachments(list, cfg.maxMediaSize);
    const attachments: Attachment[] = [];
    for (const a of plan.keep) {
      try {
        const res = await fetch(a.url);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const path = await ctx.saveMedia(new Uint8Array(await res.arrayBuffer()), extension(a.name));
        attachments.push({ type: attachmentType(a.contentType), path, name: a.name });
      } catch (err) {
        ctx.log.warn("attachment download failed", { name: a.name, error: String(err) });
        plan.notes.push(`[attachment unavailable: ${a.name}]`);
      }
    }
    return { attachments, notes: plan.notes };
  };

  const onMessage = async (ctx: ChannelContext, message: Message) => {
    if (message.author.bot || !client?.user) return;
    const botId = client.user.id;
    if (message.inGuild()) {
      if (!message.mentions.users.has(botId)) return;
      if (cfg.allowedGuilds.length > 0 && !cfg.allowedGuilds.includes(message.guildId)) return;
    }
    const { attachments, notes } = await downloadAttachments(ctx, message);
    const text = [stripMention(message.content, botId), ...notes].filter(Boolean).join("\n");
    if (!text && attachments.length === 0) return;
    if (message.channel.isSendable()) {
      message.channel.sendTyping().catch(() => undefined);
    }
    ctx.dispatch({
      chat_id: message.channelId,
      sender_id: message.author.id,
      message_id: message.id,
      text,
      attachments,
      raw: encodeRaw({ channel_id: message.channelId, message_id: message.id }),
      context: {
        guild_id: message.guildId ?? "",
        channel_id: message.channelId,
        author_name: message.author.username,
      },
      time: message.createdTimestamp,
    });
  };

  return {
    async start(ctx) {
      cfg = readConfig(ctx.config);
      client = new Client({
        intents: [GatewayIntentBits.Guilds, GatewayIntentBits.GuildMessages, GatewayIntentBits.MessageContent, GatewayIntentBits.DirectMessages],
        partials: [Partials.Channel],
      });
      client.on(Events.MessageCreate, (message) => {
        onMessage(ctx, message).catch((err) => ctx.log.error("handle message failed", { error: String(err) }));
      });
      client.on(Events.Error, (err) => ctx.log.error("discord client error", { error: String(err) }));
      const ready = new Promise<void>((resolve) => client!.once(Events.ClientReady, () => resolve()));
      try {
        await client.login(cfg.token);
      } catch (err) {
        const code = errorCode(err);
        if (code === "TokenInvalid") throw new ConfigError("Discord rejected the bot token");
        if (code === "DisallowedIntents") throw new ConfigError("enable MESSAGE CONTENT INTENT for the bot in the Discord Developer Portal");
        throw err;
      }
      await ready;
      ctx.log.info("discord bot connected", { user: client.user?.tag ?? "" });
      return {
        stop: async () => {
          await client?.destroy();
        },
      };
    },

    async send(msg: SendRequest, ctx, signal) {
      if (!client) throw new Error("discord client is not started");
      const channel = await client.channels.fetch(msg.chat_id);
      if (!channel || !channel.isSendable()) throw new Error(`discord channel ${msg.chat_id} is not sendable`);
      const replyId = msg.proactive ? undefined : (msg.reply_to?.message_id ?? decodeRaw(msg.reply_to?.raw)?.message_id);
      let reply = replyId ? { messageReference: replyId, failIfNotExists: false } : undefined;
      let lastId: string | undefined;
      for (const chunk of chunkText(msg.content ?? "")) {
        if (signal.aborted) throw new Error("send cancelled");
        const sent = await channel.send({ content: chunk, reply, allowedMentions: { parse: [], repliedUser: true } });
        lastId = sent.id;
        reply = undefined;
      }
      if (msg.media) {
        if (signal.aborted) throw new Error("send cancelled");
        const sent = await channel.send({ files: [{ attachment: msg.media.path, name: msg.media.name }], reply, allowedMentions: { parse: [], repliedUser: true } });
        lastId = sent.id;
      }
      return lastId ? { platform_message_id: lastId } : {};
    },
  };
}
