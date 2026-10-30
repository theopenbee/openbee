import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { CompassIcon } from "lucide-react"
import { Button } from "@/components/arc/button/button"
import { EmptyState } from "@/components/empty-state"
import { getAccessToken } from "@/lib/auth"

export function NotFound() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isAuthenticated = Boolean(getAccessToken())

  return (
    <main className="grid min-h-dvh place-items-center bg-background px-4 py-12">
      <h1 className="sr-only">{t("notFound.title", "Page not found")}</h1>
      <div className="w-full max-w-md min-w-0">
        <EmptyState
          icon={<CompassIcon size={24} strokeWidth={1.75} aria-hidden="true" />}
          title={t("notFound.title", "Page not found")}
          description={t("notFound.description", "The address may be mistyped, or the page was moved.")}
          action={
            <Button onClick={() => navigate(isAuthenticated ? "/" : "/login")}>
              {isAuthenticated
                ? t("notFound.backToHome", "Go to chat")
                : t("notFound.goToLogin", "Go to sign in")}
            </Button>
          }
        />
      </div>
    </main>
  )
}
