import { ChatShell } from "@/components/chat/chat-shell"
import { SettingsDialogProvider } from "@/components/settings/settings-dialog"
import { ChatEventsProvider } from "@/hooks/use-chat-events"

export function Layout() {
  return (
    <SettingsDialogProvider>
      <ChatEventsProvider>
        <ChatShell />
      </ChatEventsProvider>
    </SettingsDialogProvider>
  )
}
