export type AttachmentType = "image" | "audio" | "video" | "document" | "sticker";

export interface Attachment {
  type: AttachmentType;
  path: string;
  name?: string;
}

export interface InboundMessage {
  chat_id: string;
  sender_id: string;
  session_scope?: string;
  message_id?: string;
  text?: string;
  attachments?: Attachment[];
  raw?: string;
  context?: Record<string, unknown>;
  time?: number;
}

export interface ReplyTo {
  message_id?: string;
  raw?: string;
}

export interface SendRequest {
  session_key: string;
  chat_id: string;
  session_scope: string;
  proactive: boolean;
  reply_to?: ReplyTo;
  content?: string;
  media?: { path: string; name: string };
}

export interface SendResult {
  platform_message_id?: string;
}

export interface InitializeParams {
  protocol_version: number;
  openbee_version: string;
  channel_id: string;
  language: string;
  bot_name: string;
  config: Record<string, unknown>;
  data_dir: string;
  media_dir: string;
}

export type LogFields = Record<string, unknown>;

export interface Logger {
  debug(message: string, fields?: LogFields): void;
  info(message: string, fields?: LogFields): void;
  warn(message: string, fields?: LogFields): void;
  error(message: string, fields?: LogFields): void;
}

export interface ChannelContext {
  channelId: string;
  config: Record<string, unknown>;
  botName: string;
  language: string;
  dataDir: string;
  mediaDir: string;
  openbeeVersion: string;
  dispatch(message: InboundMessage): void;
  saveMedia(data: Uint8Array, ext: string): Promise<string>;
  log: Logger;
}

export interface ChannelHandle {
  stop?: () => void | Promise<void>;
}

export interface ChannelDefinition {
  start(ctx: ChannelContext): Promise<ChannelHandle | void> | ChannelHandle | void;
  send(message: SendRequest, ctx: ChannelContext, signal: AbortSignal): Promise<SendResult | void> | SendResult | void;
}
