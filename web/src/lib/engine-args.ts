export function parseEngineArgs(raw: string | undefined): Record<string, string> {
  if (!raw) return {}
  try {
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) return {}
    return parsed as Record<string, string>
  } catch {
    return {}
  }
}

export function stripEmptyEngineArgs(
  v: Record<string, string>,
): Record<string, string> {
  const result: Record<string, string> = {}
  for (const [k, val] of Object.entries(v)) {
    if (val.trim() !== "") result[k] = val
  }
  return result
}

export function engineArgsEqual(
  a: Record<string, string>,
  b: Record<string, string>,
): boolean {
  const keysA = Object.keys(a)
  const keysB = Object.keys(b)
  if (keysA.length !== keysB.length) return false
  return keysA.every((k) => a[k] === b[k])
}
