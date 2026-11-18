import { useEffect, useLayoutEffect, useRef, useSyncExternalStore } from "react"
import { usePlatform, formatShortcut } from "@/components/arc/shortcut-recorder/shortcut-recorder"
import { commandsVersion, hasCommand, registerCommand, SHORTCUTS, subscribeCommands, type CommandId } from "@/lib/shortcuts"

export function useCommand(id: CommandId, run: () => void, enabled = true) {
  const latest = useRef(run)
  useLayoutEffect(() => {
    latest.current = run
  })
  useEffect(() => {
    if (!enabled) return
    return registerCommand(id, () => latest.current())
  }, [id, enabled])
}

export function useHasCommand(id: CommandId): boolean {
  useSyncExternalStore(subscribeCommands, commandsVersion)
  return hasCommand(id)
}

export function useShortcutText(id: CommandId): string {
  const platform = usePlatform()
  return formatShortcut(SHORTCUTS[id], platform).text
}
