import { useState, type FormEvent } from "react"
import {
  ArrowRight,
  CircleAlert,
  Gauge,
  LoaderCircle,
} from "lucide-react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { getStoredUsername, login } from "@/lib/auth"

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
    <div className="relative min-h-screen overflow-hidden bg-background text-foreground">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 opacity-85"
        style={{
          backgroundImage: [
            "radial-gradient(circle at top left, color-mix(in oklch, var(--status-working) 18%, transparent), transparent 32%)",
            "radial-gradient(circle at bottom right, color-mix(in oklch, var(--foreground) 10%, transparent), transparent 34%)",
            "linear-gradient(180deg, color-mix(in oklch, var(--background) 96%, var(--muted) 4%), color-mix(in oklch, var(--background) 88%, var(--muted) 12%))",
            "linear-gradient(color-mix(in oklch, var(--border) 44%, transparent) 1px, transparent 1px)",
            "linear-gradient(90deg, color-mix(in oklch, var(--border) 44%, transparent) 1px, transparent 1px)",
          ].join(", "),
          backgroundSize: "100% 100%, 100% 100%, 100% 100%, 28px 28px, 28px 28px",
        }}
      />

      <div className="relative mx-auto flex min-h-screen w-full max-w-7xl flex-col px-4 pb-[max(1rem,env(safe-area-inset-bottom))] pt-[max(1rem,env(safe-area-inset-top))] sm:px-6 lg:px-8">
<div className="animate-fade-in motion-reduce:animate-none flex flex-1 flex-col">
          <main className="flex flex-1 items-center justify-center py-6 lg:py-10">
            <section
              className="relative w-full max-w-md overflow-hidden rounded-[2rem] border border-border/70 bg-card/82 p-5 shadow-[0_30px_80px_-52px_rgba(15,23,42,0.55)] backdrop-blur-sm sm:p-6 lg:p-7"
              style={{
                backgroundImage: [
                  "linear-gradient(180deg, color-mix(in oklch, var(--card) 92%, var(--muted) 8%), color-mix(in oklch, var(--card) 82%, var(--background) 18%))",
                  "radial-gradient(circle at top center, color-mix(in oklch, var(--foreground) 8%, transparent), transparent 36%)",
                ].join(", "),
              }}
            >
              <div className="absolute inset-x-8 top-0 h-px bg-gradient-to-r from-transparent via-foreground/24 to-transparent" />

              <div className="space-y-6">
                <div className="space-y-3">
                  <div className="flex items-center gap-3">
                    <div className="flex size-11 items-center justify-center rounded-2xl border border-border/70 bg-background/72 shadow-[inset_0_1px_0_rgba(255,255,255,0.08)]">
                      <Gauge className="size-4 text-foreground" />
                    </div>
                    <div className="space-y-1">
                      <p className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
                        OpenBee
                      </p>
                      <h1 className="text-2xl font-semibold tracking-[-0.04em] text-foreground sm:text-[2rem]">
                        {t("login.title")}
                      </h1>
                    </div>
                  </div>
                </div>

                {error ? (
                  <div
                    role="alert"
                    aria-live="polite"
                    className="flex items-start gap-3 rounded-[1.35rem] border border-destructive/20 bg-destructive/10 px-4 py-3 text-sm text-destructive"
                  >
                    <CircleAlert className="mt-0.5 size-4 shrink-0" />
                    <p className="leading-6">{error}</p>
                  </div>
                ) : null}

                <form onSubmit={handleSubmit} className="space-y-5" aria-busy={loading}>
                  <div className="space-y-2.5">
                    <Label htmlFor="username">{t("login.username")}</Label>
                    <Input
                      id="username"
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      autoComplete="username"
                      autoFocus={username.length === 0}
                      placeholder={t("login.usernamePlaceholder")}
                      className="h-11 rounded-xl border-border/70 bg-background/82 px-4"
                      required
                    />
                  </div>

                  <div className="space-y-2.5">
                    <Label htmlFor="password">{t("login.password")}</Label>
                    <Input
                      id="password"
                      type="password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      autoComplete="current-password"
                      placeholder={t("login.passwordPlaceholder")}
                      className="h-11 rounded-xl border-border/70 bg-background/82 px-4"
                      required
                    />
                  </div>

                  <Button
                    type="submit"
                    size="lg"
                    disabled={!canSubmit}
                    className="h-11 w-full rounded-xl px-4 text-sm font-semibold"
                  >
                    {loading ? (
                      <>
                        <LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" />
                        <span>{t("login.submitting")}</span>
                      </>
                    ) : (
                      <>
                        <span>{t("login.submit")}</span>
                        <ArrowRight className="size-4" />
                      </>
                    )}
                  </Button>
                </form>
              </div>
            </section>
          </main>
        </div>
      </div>
    </div>
  )
}
