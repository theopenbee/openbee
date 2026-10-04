import { useState, type FormEvent } from "react"
import { CircleAlert, LoaderCircle } from "lucide-react"
import { useNavigate } from "react-router-dom"
import { useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { LogoFull } from "@/components/brand/logo"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { api, ApiError } from "@/lib/api"
import { saveTokens, saveUsername } from "@/lib/auth"
import { cn, getErrorMessage } from "@/lib/utils"
import { ALERT_DESTRUCTIVE, PAGE_TITLE } from "@/lib/styles"

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
      if (err instanceof ApiError && err.status === 409) {
        setError(t("setup.errorConflict"))
      } else {
        setError(getErrorMessage(err) || t("setup.errorGeneric"))
      }
      setLoading(false)
    }
  }

  return (
    <div className="grid min-h-dvh place-items-center bg-canvas px-4 py-10 text-foreground">
      <main className="animate-fade-in motion-reduce:animate-none w-full max-w-sm">
        <section className="rounded-sm bg-card p-8 ring-1 ring-border">
          <header>
            <LogoFull className="h-7" />
            <div className="mt-6 space-y-1">
              <h1 className={PAGE_TITLE}>
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
              <Label htmlFor="username">{t("setup.username")}</Label>
              <Input
                id="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
                autoFocus
                aria-invalid={error ? true : undefined}
                placeholder={t("setup.usernamePlaceholder")}
                required
              />
            </div>

            <div className="space-y-1.5">
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
              />
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="password">{t("setup.password")}</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="new-password"
                aria-invalid={error ? true : undefined}
                placeholder={t("setup.passwordPlaceholder")}
                minLength={MIN_PASSWORD_LENGTH}
                aria-describedby="password-hint"
                required
              />
              <p id="password-hint" className="text-xs text-muted-foreground">
                {t("setup.passwordHint")}
              </p>
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
                  <span>{t("setup.submitting")}</span>
                </>
              ) : (
                <span>{t("setup.submit")}</span>
              )}
            </Button>
          </form>
        </section>
      </main>
    </div>
  )
}
