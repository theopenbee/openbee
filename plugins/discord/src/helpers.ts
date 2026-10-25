export const DISCORD_MESSAGE_LIMIT = 2000;

export function chunkText(text: string, limit = DISCORD_MESSAGE_LIMIT): string[] {
  const chunks: string[] = [];
  let rest = text;
  while (rest.length > limit) {
    const window = rest.slice(0, limit);
    const newline = window.lastIndexOf("\n");
    let cut = newline > 0 ? newline + 1 : limit;
    const code = rest.charCodeAt(cut - 1);
    if (code >= 0xd800 && code <= 0xdbff) cut -= 1;
    chunks.push(rest.slice(0, cut));
    rest = rest.slice(cut);
  }
  if (rest.length > 0) chunks.push(rest);
  return chunks;
}

export function stripMention(text: string, botId: string): string {
  return text.replace(new RegExp(`<@!?${botId}>`, "g"), "").trim();
}

export type AttachmentKind = "image" | "audio" | "video" | "document";

export function attachmentType(contentType: string | null | undefined): AttachmentKind {
  if (contentType?.startsWith("image/")) return "image";
  if (contentType?.startsWith("audio/")) return "audio";
  if (contentType?.startsWith("video/")) return "video";
  return "document";
}

export interface RawRef {
  channel_id: string;
  message_id: string;
}

export function encodeRaw(ref: RawRef): string {
  return JSON.stringify(ref);
}

export function decodeRaw(raw: string | undefined): RawRef | undefined {
  if (!raw) return undefined;
  try {
    const v = JSON.parse(raw);
    if (v && typeof v.channel_id === "string" && typeof v.message_id === "string") return v;
  } catch {
    return undefined;
  }
  return undefined;
}

export interface SizedAttachment {
  name: string;
  size: number;
}

export function planAttachments<T extends SizedAttachment>(list: T[], maxSize: number): { keep: T[]; notes: string[] } {
  const keep: T[] = [];
  const notes: string[] = [];
  for (const a of list) {
    if (a.size > maxSize) notes.push(`[attachment too large: ${a.name}]`);
    else keep.push(a);
  }
  return { keep, notes };
}
