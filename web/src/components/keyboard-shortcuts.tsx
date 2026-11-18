import { useEffect, useState } from "react"
import { matchesShortcut, usePlatform } from "@/components/arc/shortcut-recorder/shortcut-recorder"
import { QuickSearch } from "@/components/quick-search"
import { ShortcutsDialog } from "@/components/shortcuts-dialog"
import { useCommand } from "@/hooks/use-command"
import { useOpenSettings } from "@/hooks/use-settings"
import { commandFor, isEditingOutsideComposer, isOverlayOpen, runCommand, type Overlay } from "@/lib/shortcuts"

export function KeyboardShortcuts() {
  const platform = usePlatform()
  const openSettings = useOpenSettings()
  const [searchOpen, setSearchOpen] = useState(false)
  const [shortcutsOpen, setShortcutsOpen] = useState(false)

  useCommand("quickSearch", () => {
    setShortcutsOpen(false)
    setSearchOpen((open) => !open)
  })
  useCommand("showShortcuts", () => {
    setSearchOpen(false)
    setShortcutsOpen((open) => !open)
  })
  useCommand("openSettings", () => openSettings({ section: "env" }))

  useEffect(() => {
    const overlay = (): Overlay => {
      if (!isOverlayOpen()) return "none"
      return searchOpen || shortcutsOpen ? "own" : "other"
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing) return
      const id = commandFor((shortcut) => matchesShortcut(event, shortcut, platform), {
        overlay: overlay(),
        editing: isEditingOutsideComposer(event.target),
        repeat: event.repeat,
      })
      if (id && runCommand(id)) event.preventDefault()
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [platform, searchOpen, shortcutsOpen])

  return (
    <>
      <QuickSearch open={searchOpen} onOpenChange={setSearchOpen} />
      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />
    </>
  )
}
