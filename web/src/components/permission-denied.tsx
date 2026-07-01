import { useTranslation } from "react-i18next"
import { ShieldAlertIcon } from "lucide-react"

// PermissionDenied is the shared "you may not view this" state. The route guard
// renders it client-side (before any request fires) when the user lacks the
// page's permission, and the forbidden boundary renders it as a fallback when a
// query returns 403 after permissions drift mid-session.
export function PermissionDenied({ description }: { description?: string }) {
  const { t } = useTranslation()

  return (
    <div
      role="alert"
      className="animate-fade-in flex flex-col items-center justify-center py-16 text-center"
    >
      <div className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <ShieldAlertIcon className="size-6" />
      </div>
      <h2 className="mt-4 text-lg font-medium text-foreground">
        {t("permission.deniedTitle")}
      </h2>
      <p className="mt-1 max-w-sm text-sm text-muted-foreground">
        {description ?? t("permission.deniedDescription")}
      </p>
    </div>
  )
}
