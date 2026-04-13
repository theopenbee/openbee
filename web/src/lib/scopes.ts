export interface ScopeDef {
  id: string
  titleKey: string
  descriptionKey: string
}

export const KNOWN_SCOPES: ScopeDef[] = [
  {
    id: "read:workers",
    titleKey: "scopes.readWorkers.title",
    descriptionKey: "scopes.readWorkers.description",
  },
  {
    id: "read:departments",
    titleKey: "scopes.readDepartments.title",
    descriptionKey: "scopes.readDepartments.description",
  },
  {
    id: "read:tasks",
    titleKey: "scopes.readTasks.title",
    descriptionKey: "scopes.readTasks.description",
  },
]

export function parseScopes(raw: string): string[] {
  return raw ? raw.split(",").map((s) => s.trim()).filter(Boolean) : []
}

export function serializeScopes(scopes: string[]): string {
  return scopes.join(",")
}
