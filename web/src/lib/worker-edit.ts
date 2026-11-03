import { engineArgsEqual, stripEmptyEngineArgs } from "./engine-args"
import type { Engine, Worker } from "./types"

export interface WorkerEditValues {
  name: string
  description: string
  workDir: string
  engine: Engine
  engineArgs: Record<string, string>
}

export function buildWorkerUpdate(worker: Worker, values: WorkerEditValues): Record<string, unknown> | null {
  const workDir = values.workDir.trim()
  const nameChanged = values.name !== worker.name
  const descriptionChanged = values.description !== (worker.description ?? "")
  const workDirChanged = workDir !== (worker.work_dir ?? "")
  const engineChanged = values.engine !== (worker.engine ?? "")
  const engineArgsChanged = !engineArgsEqual(stripEmptyEngineArgs(values.engineArgs), worker.engine_args ?? {})

  if (!nameChanged && !descriptionChanged && !workDirChanged && !engineChanged && !engineArgsChanged) return null

  const data: Record<string, unknown> = { description: values.description, engine_args: values.engineArgs }
  if (nameChanged) data.name = values.name
  if (workDirChanged) data.work_dir = workDir
  if (engineChanged) data.engine = values.engine
  return data
}

export function followedEngine(selected: Engine, systemDefault: string | undefined, enabled: readonly Engine[]): Engine {
  if (selected) return selected
  return systemDefault && enabled.includes(systemDefault) ? systemDefault : ""
}
