import { Button } from "@/components/ui/button"
import { useCan } from "@/hooks/use-can"
import { Perm } from "@/lib/permissions"

// SystemConfigSaveButton renders a save button only when the current user holds
// system_config:write. Read-only users see no button at all, keeping the
// permission check in a single place instead of scattered {canWrite && …}
// guards at every call site.
export function SystemConfigSaveButton(props: React.ComponentProps<typeof Button>) {
  const canWrite = useCan(Perm.SystemConfigWrite)
  if (!canWrite) return null
  return <Button {...props} />
}
