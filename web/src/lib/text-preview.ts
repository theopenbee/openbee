import { basename, getFileCategory } from "@/lib/utils"

export const PREVIEW_LIMIT = 1024 * 1024
export const HIGHLIGHT_LIMIT = 128 * 1024

export type PreviewKind = "markdown" | "text" | "code"

export type TextPreview =
  | { status: "ready"; text: string }
  | { status: "too-large" }
  | { status: "unsupported" }

const MARKDOWN_EXTS = new Set(["md", "markdown"])
const TEXT_EXTS = new Set(["txt", "log", "csv"])
const LANGUAGE_ALIASES: Record<string, string> = { h: "c" }

function extension(path: string) {
  const name = basename(path)
  const dot = name.lastIndexOf(".")
  return dot < 0 ? "" : name.slice(dot + 1).toLowerCase()
}

export function previewKind(path: string): PreviewKind | null {
  const ext = extension(path)
  if (MARKDOWN_EXTS.has(ext)) return "markdown"
  if (TEXT_EXTS.has(ext)) return "text"
  if (ext && getFileCategory(path) === "code") return "code"
  return null
}

export function codeLanguage(path: string) {
  const ext = extension(path)
  return LANGUAGE_ALIASES[ext] ?? ext
}

function decode(encoding: string, data: Uint8Array) {
  try {
    return new TextDecoder(encoding, { fatal: true }).decode(data)
  } catch {
    return null
  }
}

function decodeRaw(data: Uint8Array) {
  if (data[0] === 0xff && data[1] === 0xfe) return decode("utf-16le", data)
  if (data[0] === 0xfe && data[1] === 0xff) return decode("utf-16be", data)
  if (data.includes(0)) return null
  return decode("utf-8", data) ?? decode("gb18030", data)
}

export function decodeText(data: Uint8Array): string | null {
  return decodeRaw(data)?.replace(/\r\n?/g, "\n") ?? null
}

export function fenceCode(code: string, language: string) {
  let longest = 2
  for (const run of code.matchAll(/`+/g)) longest = Math.max(longest, run[0].length)
  const fence = "`".repeat(longest + 1)
  return `${fence}${language}\n${code.replace(/\n$/, "")}\n${fence}`
}

export async function readTextPreview(res: Response): Promise<TextPreview> {
  if (Number(res.headers.get("Content-Length")) > PREVIEW_LIMIT) {
    await res.body?.cancel().catch(() => {})
    return { status: "too-large" }
  }
  const data = new Uint8Array(await res.arrayBuffer())
  if (data.byteLength > PREVIEW_LIMIT) return { status: "too-large" }
  const text = decodeText(data)
  return text === null ? { status: "unsupported" } : { status: "ready", text }
}
