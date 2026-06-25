import { useState, type FormEvent } from "react"
import { ArrowRight, CircleAlert, LoaderCircle } from "lucide-react"
import { useNavigate } from "react-router-dom"
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
    <div className="relative grid min-h-dvh place-items-center overflow-hidden bg-background px-4 py-10 text-foreground">
      {/* Tonal base: a quiet off-paper field so the white card lifts without a shadow. */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0"
        style={{
          backgroundImage:
            "linear-gradient(to bottom, var(--background), color-mix(in oklch, var(--background) 90%, var(--muted) 10%))",
        }}
      />
      {/* Blueprint grid, hairline-thin and vignetted toward the edges. */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0"
        style={{
          backgroundImage: [
            "linear-gradient(color-mix(in oklch, var(--border) 55%, transparent) 1px, transparent 1px)",
            "linear-gradient(90deg, color-mix(in oklch, var(--border) 55%, transparent) 1px, transparent 1px)",
          ].join(", "),
          backgroundSize: "32px 32px, 32px 32px",
          maskImage:
            "radial-gradient(ellipse 80% 70% at 50% 42%, black, transparent 78%)",
          WebkitMaskImage:
            "radial-gradient(ellipse 80% 70% at 50% 42%, black, transparent 78%)",
        }}
      />

      <main className="animate-fade-in motion-reduce:animate-none relative w-full max-w-sm">
        <section className="rounded-sm border border-border bg-card p-6 ring-1 ring-foreground/5 sm:p-8">
          <header className="space-y-5">
            <LogoFull className="h-9" />
            <div className="space-y-1.5">
              <h1 className="text-2xl font-semibold tracking-tight">
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
              className={cn(ALERT_DESTRUCTIVE, "mt-6 flex items-start gap-2.5 px-3.5 py-3")}
            >
              <CircleAlert aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
              <p className="leading-6">{error}</p>
            </div>
          ) : null}

          <form
            onSubmit={handleSubmit}
            className="mt-6 space-y-5"
            aria-busy={loading}
          >
            <div className="space-y-2">
              <Label htmlFor="username">{t("login.username")}</Label>
              <Input
                id="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
                autoFocus={username.length === 0}
                aria-invalid={error ? true : undefined}
                placeholder={t("login.usernamePlaceholder")}
                className="h-11 px-3.5"
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">{t("login.password")}</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                aria-invalid={error ? true : undefined}
                placeholder={t("login.passwordPlaceholder")}
                className="h-11 px-3.5"
                required
              />
            </div>

            <Button
              type="submit"
              disabled={!canSubmit}
              className="h-11 w-full text-sm font-semibold"
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
                <>
                  <span>{t("login.submit")}</span>
                  <ArrowRight aria-hidden="true" className="size-4" />
                </>
              )}
            </Button>
          </form>
        </section>
      </main>
    </div>
  )
}
