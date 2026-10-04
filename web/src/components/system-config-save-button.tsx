import { LoaderCircle } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useCan } from "@/hooks/use-can"
import { Perm } from "@/lib/permissions"

// SystemConfigSaveButton renders a save button only when the current user holds
// system_config:write. Read-only users see no button at all, keeping the
// permission check in a single place instead of scattered {canWrite && …}
// guards at every call site. Pass `saving` while the mutation is in flight to
// show a spinner and hold the button disabled.
export function SystemConfigSaveButton({
  saving = false,
  disabled,
  children,
  ...props
}: React.ComponentProps<typeof Button> & { saving?: boolean }) {
  const canWrite = useCan(Perm.SystemConfigWrite)
  if (!canWrite) return null
  return (
    <Button {...props} disabled={disabled || saving} aria-busy={saving || undefined}>
      {saving && (
        <LoaderCircle aria-hidden="true" className="size-4 animate-spin motion-reduce:animate-none" />
      )}
      {children}
    </Button>
  )
}
