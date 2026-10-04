import { useState, type FormEvent } from "react"
import { CircleAlert, LoaderCircle } from "lucide-react"
import { useNavigate } from "react-router-dom"
import { useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { LogoFull } from "@/components/brand/logo"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { getStoredUsername, login } from "@/lib/auth"
import { cn } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"

export function Login() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [username, setUsername] = useState(() => getStoredUsername() ?? "")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)
  const canSubmit = username.trim().length > 0 && password.length > 0 && !loading

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!canSubmit) return

    setError("")
    setLoading(true)

    const result = await login(username, password)
    setLoading(false)

    if (result.success) {
      // Backstop against stale per-user cache: any logout path that funnels
      // through this form (including a forced 401 redirect) starts the new
      // session with an empty cache, so we never read the previous user's
      // permissions/me.
      queryClient.clear()
      navigate("/", { replace: true })
    } else if (result.status === 401) {
      setError(t("login.error401"))
    } else if (result.status === 429) {
      setError(t("login.error429"))
    } else {
      setError(t("login.errorGeneric"))
    }
  }

  return (
    <div className="grid min-h-dvh place-items-center bg-canvas px-4 py-10 text-foreground">
      <main className="animate-fade-in motion-reduce:animate-none w-full max-w-sm">
        <section className="rounded-sm bg-card p-8 ring-1 ring-border">
          <header>
            <LogoFull className="h-7" />
            <div className="mt-6 space-y-1">
              <h1 className="text-xl leading-7 font-semibold tracking-[-0.015em] text-strong">
                {t("login.title")}
              </h1>
              <p className="text-sm text-muted-foreground">
                {t("login.eyebrow")}
              </p>
            </div>
          </header>

          {error ? (
            <div
              role="alert"
              aria-live="polite"
              className={cn(ALERT_DESTRUCTIVE, "mt-6 flex items-start gap-2.5 px-3 py-2.5")}
            >
              <CircleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
              <p className="leading-5">{error}</p>
            </div>
          ) : null}

          <form
            onSubmit={handleSubmit}
            className="mt-6 space-y-4"
            aria-busy={loading}
          >
            <div className="space-y-1.5">
              <Label htmlFor="username">{t("login.username")}</Label>
              <Input
                id="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
                autoFocus={username.length === 0}
                aria-invalid={error ? true : undefined}
                placeholder={t("login.usernamePlaceholder")}
                required
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="password">{t("login.password")}</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                aria-invalid={error ? true : undefined}
                placeholder={t("login.passwordPlaceholder")}
                required
              />
            </div>

            <Button
              type="submit"
              disabled={!canSubmit}
              className="mt-2 w-full"
            >
              {loading ? (
                <>
                  <LoaderCircle
                    aria-hidden="true"
                    className="size-4 animate-spin motion-reduce:animate-none"
                  />
                  <span>{t("login.submitting")}</span>
                </>
              ) : (
                <span>{t("login.submit")}</span>
              )}
            </Button>
          </form>
        </section>
      </main>
    </div>
  )
}
