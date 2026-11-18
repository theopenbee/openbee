import { useEffect, useState } from "react"
import { matchesShortcut, usePlatform } from "@/components/arc/shortcut-recorder/shortcut-recorder"
import { QuickSearch } from "@/components/quick-search"
import { ShortcutsDialog } from "@/components/shortcuts-dialog"
import { useCommand } from "@/hooks/use-command"
import { useOpenSettings } from "@/hooks/use-settings"
import { commandFor, isDialogOpen, runCommand } from "@/lib/shortcuts"

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
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing) return
      const id = commandFor((shortcut) => matchesShortcut(event, shortcut, platform), isDialogOpen())
      if (id && runCommand(id)) event.preventDefault()
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [platform])

  return (
    <>
      <QuickSearch open={searchOpen} onOpenChange={setSearchOpen} />
      <ShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />
    </>
  )
}
