import type { CatalogPlugin, LocalizedText, PluginConfigField, PluginOperation, PluginState } from "./types"

export type FormValue = string | boolean
export type FormValues = Record<string, FormValue>
export type MarketAction = "install" | "installing" | "configure" | "update" | "incompatible"

const INT_PATTERN = /^-?\d+$/
const LINK_PATTERN = /^https?:\/\/\S+$/

export function localize(text: LocalizedText | undefined, lang: string): string {
  if (!text) return ""
  return text[lang] ?? text[lang.split("-")[0]] ?? text.en ?? Object.values(text)[0] ?? ""
}

export function randomHex(bytes = 8): string {
  const buf = new Uint8Array(bytes)
  crypto.getRandomValues(buf)
  return Array.from(buf, (b) => b.toString(16).padStart(2, "0")).join("")
}

function hasValue(config: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(config, key) && config[key] !== null && config[key] !== undefined
}

function splitLines(text: string): string[] {
  return text.split("\n").map((s) => s.trim()).filter(Boolean)
}

export function initialFormValues(fields: PluginConfigField[], config: Record<string, unknown>): FormValues {
  const values: FormValues = {}
  for (const f of fields) {
    const saved = hasValue(config, f.key)
    const cur = config[f.key]
    switch (f.type) {
      case "secret":
        values[f.key] = ""
        break
      case "bool":
        values[f.key] = saved ? Boolean(cur) : Boolean(f.default ?? false)
        break
      case "int":
        values[f.key] = saved ? String(cur) : ""
        break
      case "list":
        values[f.key] = Array.isArray(cur) ? cur.map(String).join("\n") : ""
        break
      default:
        if (saved) values[f.key] = String(cur)
        else if (f.random_default && !Object.prototype.hasOwnProperty.call(config, f.key)) values[f.key] = randomHex()
        else values[f.key] = ""
    }
  }
  return values
}

export function invalidIntFields(fields: PluginConfigField[], values: FormValues): string[] {
  return fields
    .filter((f) => f.type === "int")
    .filter((f) => {
      const v = String(values[f.key] ?? "").trim()
      return v !== "" && !INT_PATTERN.test(v)
    })
    .map((f) => f.key)
}

export function buildSettingsPayload(
  fields: PluginConfigField[],
  config: Record<string, unknown>,
  values: FormValues,
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  for (const f of fields) {
    const saved = hasValue(config, f.key)
    const cur = config[f.key]
    const raw = values[f.key]
    switch (f.type) {
      case "secret": {
        const v = String(raw ?? "").trim()
        if (v !== "") out[f.key] = v
        break
      }
      case "bool": {
        const base = saved ? Boolean(cur) : Boolean(f.default ?? false)
        if (Boolean(raw) !== base) out[f.key] = Boolean(raw)
        break
      }
      case "int": {
        const v = String(raw ?? "").trim()
        if (v === "") {
          if (saved) out[f.key] = null
        } else if (!saved || Number(v) !== Number(cur)) {
          out[f.key] = Number(v)
        }
        break
      }
      case "list": {
        const next = splitLines(String(raw ?? ""))
        const prev = Array.isArray(cur) ? cur.map(String) : []
        if (saved ? JSON.stringify(next) !== JSON.stringify(prev) : next.length > 0) out[f.key] = next
        break
      }
      default: {
        const v = String(raw ?? "")
        if (saved) {
          if (v !== String(cur)) out[f.key] = v
        } else if (f.random_default && !Object.prototype.hasOwnProperty.call(config, f.key)) {
          out[f.key] = v
        } else if (v !== "") {
          out[f.key] = v
        }
      }
    }
  }
  return out
}

export function filterCatalog(plugins: CatalogPlugin[], query: string, tag: string | null): CatalogPlugin[] {
  const q = query.trim().toLowerCase()
  return plugins.filter((p) => {
    if (tag && !p.tags.includes(tag)) return false
    if (!q) return true
    const haystack = [p.id, ...Object.values(p.name ?? {}), ...Object.values(p.description ?? {})]
    return haystack.some((s) => s.toLowerCase().includes(q))
  })
}

export function marketAction(p: CatalogPlugin, pending: boolean): MarketAction {
  if (pending) return "installing"
  if (p.installed) return p.update_available && p.installable ? "update" : "configure"
  return p.installable ? "install" : "incompatible"
}

export function specPackage(spec: string): string {
  const at = spec.lastIndexOf("@")
  return at > 0 ? spec.slice(0, at) : spec
}

export function pendingInstalls(ops: PluginOperation[]): Record<string, PluginOperation> {
  const out: Record<string, PluginOperation> = {}
  for (const op of ops) {
    if (op.kind !== "install" || !op.spec) continue
    if (op.status !== "queued" && op.status !== "running") continue
    const pkg = specPackage(op.spec)
    if (!out[pkg]) out[pkg] = op
  }
  return out
}

export function latestLink(lines: string[]): string | null {
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i].trim()
    if (LINK_PATTERN.test(line)) return line
  }
  return null
}

export function isTransitional(state: PluginState): boolean {
  return state === "starting" || state === "retrying"
}

export function formatElapsed(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, "0")}`
}
