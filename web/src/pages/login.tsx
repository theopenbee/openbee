import { useRef, useState, type FormEvent } from "react"
import { useNavigate } from "react-router-dom"
import { useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Input } from "@/components/arc/input/input"
import { AuthLayout } from "@/components/auth-layout"
import { AuthPasswordField } from "@/components/auth-password-field"
import { getStoredUsername, login } from "@/lib/auth"

interface FieldErrors {
  username?: string
  password?: string
}

export function Login() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [username, setUsername] = useState(() => getStoredUsername() ?? "")
  const [password, setPassword] = useState("")
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [error, setError] = useState("")
  const [loading, setLoading] = useState(false)
  const [focusPassword] = useState(() => username.length > 0)
  const usernameRef = useRef<HTMLInputElement>(null)
  const passwordRef = useRef<HTMLInputElement>(null)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (loading) return

    const nextErrors: FieldErrors = {
      username: username.trim() ? undefined : t("login.usernameRequired", "Enter your username"),
      password: password ? undefined : t("login.passwordRequired", "Enter your password"),
    }
    if (nextErrors.username || nextErrors.password) {
      setFieldErrors(nextErrors)
      if (nextErrors.username) usernameRef.current?.focus()
      else passwordRef.current?.focus()
      return
    }

    setFieldErrors({})
    setError("")
    setLoading(true)

    const result = await login(username, password)

    if (result.success) {
      queryClient.clear()
      navigate("/", { replace: true })
      return
    }

    setLoading(false)
    if (result.status === 401) {
      setError(t("login.error401", "The username or password is incorrect."))
      passwordRef.current?.select()
    } else if (result.status === 429) {
      setError(t("login.error429", "Too many attempts. Wait a minute and try again."))
    } else {
      setError(t("login.errorGeneric", "Check your connection and try again."))
    }
  }

  return (
    <AuthLayout
      title={t("login.title", "Sign in")}
      description={t("login.description", "Use your OpenBee administrator account.")}
    >
      <form onSubmit={handleSubmit} noValidate aria-busy={loading} className="grid gap-4">
        {error ? (
          <Alert tone="danger" title={t("login.errorTitle", "Could not sign in")}>
            {error}
          </Alert>
        ) : null}

        <Input
          ref={usernameRef}
          label={t("login.username", "Username")}
          name="username"
          value={username}
          onChange={(e) => {
            setUsername(e.target.value)
            setFieldErrors((prev) => ({ ...prev, username: undefined }))
          }}
          error={fieldErrors.username}
          autoComplete="username"
          autoCapitalize="none"
          spellCheck={false}
          autoFocus={!focusPassword}
        />

        <AuthPasswordField
          ref={passwordRef}
          label={t("login.password", "Password")}
          name="password"
          value={password}
          onChange={(e) => {
            setPassword(e.target.value)
            setFieldErrors((prev) => ({ ...prev, password: undefined }))
          }}
          error={fieldErrors.password}
          autoComplete="current-password"
          autoFocus={focusPassword}
        />

        <Button type="submit" loading={loading} className="mt-2 w-full">
          {t("login.submit", "Sign in")}
        </Button>
      </form>
    </AuthLayout>
  )
}
