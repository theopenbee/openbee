export const SCOPE_READ_WORKERS = "read:workers"
export const SCOPE_READ_DEPARTMENTS = "read:departments"
export const SCOPE_READ_TASKS = "read:tasks"
export const SCOPE_READ_MESSAGES = "read:messages"
export const SCOPE_READ_EXECUTIONS = "read:executions"

export interface ScopeDef {
  id: string
  titleKey: string
  descriptionKey: string
}

export const KNOWN_SCOPES: ScopeDef[] = [
  {
    id: SCOPE_READ_WORKERS,
    titleKey: "scopes.readWorkers.title",
    descriptionKey: "scopes.readWorkers.description",
  },
  {
    id: SCOPE_READ_DEPARTMENTS,
    titleKey: "scopes.readDepartments.title",
    descriptionKey: "scopes.readDepartments.description",
  },
  {
    id: SCOPE_READ_TASKS,
    titleKey: "scopes.readTasks.title",
    descriptionKey: "scopes.readTasks.description",
  },
  {
    id: SCOPE_READ_MESSAGES,
    titleKey: "scopes.readMessages.title",
    descriptionKey: "scopes.readMessages.description",
  },
  {
    id: SCOPE_READ_EXECUTIONS,
    titleKey: "scopes.readExecutions.title",
    descriptionKey: "scopes.readExecutions.description",
  },
]

export function parseScopes(raw: string): string[] {
  return raw ? raw.split(",").map((s) => s.trim()).filter(Boolean) : []
}

export function serializeScopes(scopes: string[]): string {
  return scopes.join(",")
}
