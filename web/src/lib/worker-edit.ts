import { engineArgsEqual, stripEmptyEngineArgs } from "./engine-args"
import { changedEngineModels, engineModel } from "./engine-models"
import type { Engine, Worker } from "./types"

export interface WorkerEditValues {
  name: string
  description: string
  workDir: string
  engine: Engine
  engineArgs: Record<string, string>
  engineModels: Record<string, string>
  avatar: string
}

export function buildWorkerUpdate(
  worker: Worker,
  values: WorkerEditValues,
  initialEngineModels: Record<string, string> = worker.engine_models ?? {},
): Record<string, unknown> | null {
  const workDir = values.workDir.trim()
  const nameChanged = values.name !== worker.name
  const descriptionChanged = values.description !== (worker.description ?? "")
  const workDirChanged = workDir !== (worker.work_dir ?? "")
  const engineChanged = values.engine !== (worker.engine ?? "")
  const engineArgsChanged = !engineArgsEqual(stripEmptyEngineArgs(values.engineArgs), worker.engine_args ?? {})
  const engineModels = changedEngineModels(initialEngineModels, values.engineModels)
  const avatarChanged = values.avatar !== (worker.avatar ?? "")

  if (!nameChanged && !descriptionChanged && !workDirChanged && !engineChanged && !engineArgsChanged && !engineModels && !avatarChanged) {
    return null
  }

  const data: Record<string, unknown> = { description: values.description, engine_args: values.engineArgs }
  if (nameChanged) data.name = values.name
  if (workDirChanged) data.work_dir = workDir
  if (engineChanged) data.engine = values.engine
  if (engineModels) data.engine_models = engineModels
  if (avatarChanged) data.avatar = values.avatar
  return data
}

export function followedEngine(selected: Engine, systemDefault: string | undefined, enabled: readonly Engine[]): Engine {
  if (selected) return selected
  return systemDefault && enabled.includes(systemDefault) ? systemDefault : ""
}

export interface ProfileEngine {
  engine: Engine
  model: string
}

export function profileEngine(
  worker: Worker,
  systemDefault: string | undefined,
  enabled: readonly Engine[],
  defaultModels: Record<string, string> = {},
): ProfileEngine | null {
  const modelFor = (engine: Engine) => engineModel(worker.engine_models, engine) || engineModel(defaultModels, engine)
  if (worker.engine) return { engine: worker.engine, model: modelFor(worker.engine) }
  const followed = followedEngine("", systemDefault, enabled)
  const model = modelFor(followed)
  return model ? { engine: followed, model } : null
}
