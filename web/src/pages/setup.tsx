import { useState, type FormEvent } from "react"
import { ArrowRight, CircleAlert, LoaderCircle } from "lucide-react"
import { useNavigate } from "react-router-dom"
import { useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { LogoFull } from "@/components/brand/logo"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { api } from "@/lib/api"
import { saveTokens, saveUsername } from "@/lib/auth"
import { cn } from "@/lib/utils"
import { ALERT_DESTRUCTIVE } from "@/lib/styles"

const MIN_PASSWORD_LENGTH = 6

export function Setup() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [username, setUsername] = useState("")
  const [displayName, setDisplayName] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)

  const canSubmit =
    username.trim().length > 0 && password.length >= MIN_PASSWORD_LENGTH && !loading

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (loading) return

    if (password.length < MIN_PASSWORD_LENGTH) {
      setError(t("setup.errorPasswordLength"))
      return
    }

    setError("")
    setLoading(true)

    try {
      const trimmedDisplay = displayName.trim()
      const tokens = await api.setup.create({
        username: username.trim(),
        password,
        display_name: trimmedDisplay.length > 0 ? trimmedDisplay : undefined,
      })
      saveTokens(tokens.access_token, tokens.refresh_token)
      saveUsername(username.trim())
      // The AuthGuard caches the setup probe with staleTime: Infinity, so the
      // first-run value (initialized: false) would otherwise bounce us straight
      // back here. Seed the freshly-true status synchronously before navigating.
      queryClient.setQueryData(["setup", "status"], { initialized: true })
      navigate("/", { replace: true })
    } catch (err) {
      const message = err instanceof Error ? err.message : ""
      if (/409|exist|initial/i.test(message)) {
        setError(t("setup.errorConflict"))
      } else {
        setError(message || t("setup.errorGeneric"))
      }
      setLoading(false)
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
                {t("setup.title")}
              </h1>
              <p className="text-sm text-muted-foreground">
                {t("setup.description")}
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
              <Label htmlFor="username">{t("setup.username")}</Label>
              <Input
                id="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
                autoFocus
                aria-invalid={error ? true : undefined}
                placeholder={t("setup.usernamePlaceholder")}
                className="h-11 px-3.5"
                required
              />
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="displayName">{t("setup.displayName")}</Label>
                <span className="text-xs text-muted-foreground">
                  {t("setup.displayNameOptional")}
                </span>
              </div>
              <Input
                id="displayName"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                autoComplete="name"
                placeholder={t("setup.displayNamePlaceholder")}
                className="h-11 px-3.5"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="password">{t("setup.password")}</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="new-password"
                aria-invalid={error ? true : undefined}
                placeholder={t("setup.passwordPlaceholder")}
                className="h-11 px-3.5"
                minLength={MIN_PASSWORD_LENGTH}
                required
              />
              <p className="text-xs text-muted-foreground">{t("setup.passwordHint")}</p>
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
                  <span>{t("setup.submitting")}</span>
                </>
              ) : (
                <>
                  <span>{t("setup.submit")}</span>
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
