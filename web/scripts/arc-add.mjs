import { execFileSync } from "node:child_process"
import { existsSync, readdirSync, readFileSync, statSync, writeFileSync } from "node:fs"
import { join } from "node:path"
import ts from "typescript"

const ARC_DIR = "src/components/arc"
const KEEP = /@ts-expect-error|@ts-ignore|@ts-nocheck/

function listFiles(dir) {
  if (!existsSync(dir)) return []
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    return statSync(path).isDirectory() ? listFiles(path) : [path]
  })
}

function snapshot() {
  return new Map(listFiles(ARC_DIR).map((file) => [file, statSync(file).mtimeMs]))
}

function commentRanges(text, file) {
  const kind = file.endsWith(".tsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS
  const source = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true, kind)
  const ranges = []
  const seen = new Set()
  const collect = (pos) => {
    for (const range of [...(ts.getLeadingCommentRanges(text, pos) ?? []), ...(ts.getTrailingCommentRanges(text, pos) ?? [])]) {
      if (seen.has(range.pos)) continue
      seen.add(range.pos)
      if (!KEEP.test(text.slice(range.pos, range.end))) ranges.push([range.pos, range.end])
    }
  }
  const visit = (node) => {
    if (node.kind === ts.SyntaxKind.JsxText) return
    if (node.kind === ts.SyntaxKind.JsxExpression && !node.expression) {
      ranges.push([node.getStart(source), node.end])
      return
    }
    const children = node.getChildren(source)
    if (children.length === 0) collect(node.pos)
    else children.forEach(visit)
  }
  visit(source)
  collect(source.endOfFileToken.pos)
  return ranges
}

function cssCommentRanges(text) {
  return [...text.matchAll(/\/\*[\s\S]*?\*\//g)].map((match) => [match.index, match.index + match[0].length])
}

function removeRanges(text, ranges) {
  const merged = []
  for (const range of ranges.sort((a, b) => a[0] - b[0])) {
    const last = merged.at(-1)
    if (last && range[0] <= last[1]) last[1] = Math.max(last[1], range[1])
    else merged.push([...range])
  }
  for (const range of merged) {
    let start = range[0]
    let end = range[1]
    while (start > 0 && (text[start - 1] === " " || text[start - 1] === "\t")) start--
    while (end < text.length && (text[end] === " " || text[end] === "\t")) end++
    const atLineStart = start === 0 || text[start - 1] === "\n"
    const atLineEnd = end === text.length || text[end] === "\n"
    if (atLineStart && atLineEnd) {
      range[0] = start
      range[1] = end < text.length ? end + 1 : end
    } else if (atLineEnd) {
      range[0] = start
    }
  }
  let out = ""
  let cursor = 0
  for (const [start, end] of merged) {
    if (start < cursor) {
      cursor = Math.max(cursor, end)
      continue
    }
    out += text.slice(cursor, start)
    cursor = end
  }
  return out + text.slice(cursor)
}

function stripComments(file) {
  const text = readFileSync(file, "utf8")
  const ranges = /\.tsx?$/.test(file) ? commentRanges(text, file) : file.endsWith(".css") ? cssCommentRanges(text) : []
  if (ranges.length) writeFileSync(file, removeRanges(text, ranges))
}

const ids = process.argv.slice(2)
if (ids.length === 0) {
  console.error("usage: pnpm arc:add <id> [<id> ...]")
  process.exit(1)
}

const before = snapshot()
execFileSync("pnpm", ["dlx", "shadcn@latest", "add", ...ids.map((id) => `@uiarc/${id}`), "--yes"], { stdio: "inherit" })
const changed = [...snapshot()].filter(([file, mtime]) => before.get(file) !== mtime).map(([file]) => file)
changed.forEach(stripComments)
console.log(`arc:add stripped comments from ${changed.length} file(s)`)
