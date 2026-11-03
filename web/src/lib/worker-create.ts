export interface AdvancedWorkerSettings {
  constraints: string
  workDir: string
  engineArgs: string
  scopes: readonly string[]
}

export function countAdvancedSettings({ constraints, workDir, engineArgs, scopes }: AdvancedWorkerSettings): number {
  const filled = [constraints.trim() !== "", workDir.trim() !== "", engineArgs.trim() !== "", scopes.length > 0]
  return filled.filter(Boolean).length
}
