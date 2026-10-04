import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { getAccessToken } from "@/lib/auth"

export function NotFound() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const isAuthenticated = Boolean(getAccessToken())

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center bg-canvas px-6 text-center">
      <h1 className="flex items-center gap-2 text-base font-semibold text-strong">
        {t("notFound.title")}
        <Badge variant="secondary" className="font-mono tabular-nums">
          {t("notFound.code")}
        </Badge>
      </h1>
      <p className="mt-1 max-w-md text-sm text-balance text-muted-foreground">{t("notFound.description")}</p>
      <Button className="mt-5" onClick={() => navigate(isAuthenticated ? "/" : "/login")}>
        {isAuthenticated ? t("notFound.backToHome") : t("notFound.goToLogin")}
      </Button>
    </div>
  )
}
