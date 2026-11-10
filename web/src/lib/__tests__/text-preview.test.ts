import { describe, expect, it } from "vitest"
import {
  codeLanguage,
  decodeText,
  fenceCode,
  PREVIEW_LIMIT,
  previewKind,
  readTextPreview,
} from "../text-preview"

function bytes(...values: number[]) {
  return new Uint8Array(values)
}

describe("previewKind", () => {
  it("renders markdown files", () => {
    expect(previewKind("abc_README.md")).toBe("markdown")
    expect(previewKind("notes.MARKDOWN")).toBe("markdown")
  })

  it("shows plain text files as text", () => {
    expect(previewKind("a.txt")).toBe("text")
    expect(previewKind("server.log")).toBe("text")
    expect(previewKind("rows.csv")).toBe("text")
  })

  it("highlights code and config files", () => {
    expect(previewKind("main.go")).toBe("code")
    expect(previewKind("config.yaml")).toBe("code")
    expect(previewKind("data.json")).toBe("code")
  })

  it("skips other files", () => {
    expect(previewKind("photo.png")).toBeNull()
    expect(previewKind("report.pdf")).toBeNull()
    expect(previewKind("archive.zip")).toBeNull()
    expect(previewKind("README")).toBeNull()
    expect(previewKind("folder.md/file")).toBeNull()
  })
})

describe("codeLanguage", () => {
  it("uses the extension as the language", () => {
    expect(codeLanguage("main.go")).toBe("go")
    expect(codeLanguage("App.TSX")).toBe("tsx")
  })

  it("maps headers to C", () => {
    expect(codeLanguage("lib.h")).toBe("c")
  })
})

describe("decodeText", () => {
  it("decodes UTF-8 and drops the byte order mark", () => {
    const text = new TextEncoder().encode("﻿你好 world")
    expect(decodeText(text)).toBe("你好 world")
  })

  it("falls back to GB18030", () => {
    expect(decodeText(bytes(0xc4, 0xe3, 0xba, 0xc3))).toBe("你好")
  })

  it("decodes UTF-16 with a byte order mark", () => {
    expect(decodeText(bytes(0xff, 0xfe, 0x60, 0x4f, 0x7d, 0x59))).toBe("你好")
    expect(decodeText(bytes(0xfe, 0xff, 0x4f, 0x60, 0x59, 0x7d))).toBe("你好")
  })

  it("normalises line endings", () => {
    expect(decodeText(new TextEncoder().encode("a\r\nb\rc\n"))).toBe("a\nb\nc\n")
  })

  it("rejects binary data", () => {
    expect(decodeText(bytes(0x89, 0x50, 0x4e, 0x47, 0x00, 0x01))).toBeNull()
  })

  it("rejects bytes that no encoding accepts", () => {
    expect(decodeText(bytes(0x61, 0xff))).toBeNull()
  })

  it("accepts an empty file", () => {
    expect(decodeText(bytes())).toBe("")
  })
})

describe("fenceCode", () => {
  it("wraps code in a fenced block", () => {
    expect(fenceCode("let a = 1\n", "ts")).toBe("```ts\nlet a = 1\n```")
  })

  it("uses a fence longer than any backtick run in the code", () => {
    expect(fenceCode("a ```` b", "md")).toBe("`````md\na ```` b\n`````")
  })
})

describe("readTextPreview", () => {
  it("reads text responses", async () => {
    expect(await readTextPreview(new Response("# Title"))).toEqual({ status: "ready", text: "# Title" })
  })

  it("stops at the size limit without reading the body", async () => {
    let pulled = false
    const body = new ReadableStream({ pull() { pulled = true } })
    const res = new Response(body, { headers: { "Content-Length": String(PREVIEW_LIMIT + 1) } })
    expect(await readTextPreview(res)).toEqual({ status: "too-large" })
    expect(pulled).toBe(false)
  })

  it("checks the size when the length header is missing", async () => {
    const res = new Response(new Uint8Array(PREVIEW_LIMIT + 1).fill(0x61))
    expect(await readTextPreview(res)).toEqual({ status: "too-large" })
  })

  it("accepts a file of exactly the limit", async () => {
    const res = new Response(new Uint8Array(PREVIEW_LIMIT).fill(0x61))
    const preview = await readTextPreview(res)
    expect(preview.status).toBe("ready")
  })

  it("reports binary files as unsupported", async () => {
    expect(await readTextPreview(new Response(bytes(0x00, 0x01)))).toEqual({ status: "unsupported" })
  })
})
