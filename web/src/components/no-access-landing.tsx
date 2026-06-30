import { useTranslation } from "react-i18next"
import { LockIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { clearTokens } from "@/lib/auth"

// NoAccessLanding is the safety net for an account with no reachable page at
// all. The home resolver shows it instead of redirecting, so an account with
// zero permissions never falls into a redirect loop. With the current nav (chat
// is ungated) this is unreachable, but it keeps a future, stricter config safe.
export function NoAccessLanding() {
  const { t } = useTranslation()

  function logout() {
    clearTokens()
    window.location.hash = "#/login"
  }

  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center px-6 text-center">
      <div className="flex size-14 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <LockIcon className="size-7" />
      </div>
      <h1 className="mt-5 text-xl font-semibold text-foreground">
        {t("permission.noAccessTitle")}
      </h1>
      <p className="mt-2 max-w-md text-sm text-muted-foreground">
        {t("permission.noAccessDescription")}
      </p>
      <Button variant="outline" className="mt-6" onClick={logout}>
        {t("permission.logout")}
      </Button>
    </div>
  )
}
