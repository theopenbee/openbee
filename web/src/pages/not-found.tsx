import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Button } from "@/components/ui/button"
import { getAccessToken } from "@/lib/auth"

export function NotFound() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isAuthenticated = Boolean(getAccessToken())

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center gap-4 bg-background text-center">
      <p className="text-8xl font-bold text-muted-foreground">{t("notFound.code")}</p>
      <h1 className="text-2xl font-semibold">{t("notFound.title")}</h1>
      <p className="text-muted-foreground">{t("notFound.description")}</p>
      <Button onClick={() => navigate(isAuthenticated ? "/" : "/login")}>
        {isAuthenticated ? t("notFound.backToHome") : t("notFound.goToLogin")}
      </Button>
    </div>
  )
}
