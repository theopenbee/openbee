import { useRef, useState, type FormEvent } from "react"
import { useNavigate } from "react-router-dom"
import { useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Input } from "@/components/arc/input/input"
import { AuthLayout } from "@/components/auth-layout"
import { AuthPasswordField } from "@/components/auth-password-field"
import { api, ApiError } from "@/lib/api"
import { saveTokens, saveUsername } from "@/lib/auth"
import { getErrorMessage } from "@/lib/utils"

const MIN_PASSWORD_LENGTH = 6

interface FieldErrors {
  username?: string
  password?: string
}

interface FormError {
  message: string
  conflict: boolean
}

export function Setup() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [username, setUsername] = useState("")
  const [displayName, setDisplayName] = useState("")
  const [password, setPassword] = useState("")
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [error, setError] = useState<FormError | null>(null)
  const [loading, setLoading] = useState(false)
  const usernameRef = useRef<HTMLInputElement>(null)
  const passwordRef = useRef<HTMLInputElement>(null)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (loading) return

    const nextErrors: FieldErrors = {
      username: username.trim() ? undefined : t("setup.usernameRequired", "Enter a username"),
      password:
        password.length >= MIN_PASSWORD_LENGTH
          ? undefined
          : t("setup.errorPasswordLength", "Use at least 6 characters"),
    }
    if (nextErrors.username || nextErrors.password) {
      setFieldErrors(nextErrors)
      if (nextErrors.username) usernameRef.current?.focus()
      else passwordRef.current?.focus()
      return
    }

    setFieldErrors({})
    setError(null)
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
      queryClient.setQueryData(["setup", "status"], { initialized: true })
      navigate("/", { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError({
          message: t("setup.errorConflict", "An admin account already exists. Sign in with it instead."),
          conflict: true,
        })
      } else {
        setError({
          message: getErrorMessage(err) || t("setup.errorGeneric", "Check your connection and try again."),
          conflict: false,
        })
      }
      setLoading(false)
    }
  }

  return (
    <AuthLayout
      title={t("setup.title", "Create the admin account")}
      description={t(
        "setup.description",
        "OpenBee has no account yet. The account you create here signs in to this console.",
      )}
    >
      <form onSubmit={handleSubmit} noValidate aria-busy={loading} className="grid gap-4">
        {error ? (
          <Alert tone="danger" title={t("setup.errorTitle", "Could not create the account")}>
            <span className="block">{error.message}</span>
            {error.conflict ? (
              <span className="mt-3 block">
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  onClick={() => navigate("/login", { replace: true })}
                >
                  {t("setup.goToLogin", "Go to sign in")}
                </Button>
              </span>
            ) : null}
          </Alert>
        ) : null}

        <Input
          ref={usernameRef}
          label={t("setup.username", "Username")}
          name="username"
          value={username}
          onChange={(e) => {
            setUsername(e.target.value)
            setFieldErrors((prev) => ({ ...prev, username: undefined }))
          }}
          error={fieldErrors.username}
          placeholder={t("setup.usernamePlaceholder", "admin")}
          autoComplete="username"
          autoCapitalize="none"
          spellCheck={false}
          autoFocus
        />

        <Input
          label={t("setup.displayName", "Display name")}
          name="displayName"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          description={t("setup.displayNameOptional", "Optional")}
          placeholder={t("setup.displayNamePlaceholder", "Administrator")}
          autoComplete="name"
        />

        <AuthPasswordField
          ref={passwordRef}
          label={t("setup.password", "Password")}
          name="password"
          value={password}
          onChange={(e) => {
            setPassword(e.target.value)
            setFieldErrors((prev) => ({ ...prev, password: undefined }))
          }}
          description={fieldErrors.password ? undefined : t("setup.passwordHint", "Use at least 6 characters.")}
          error={fieldErrors.password}
          autoComplete="new-password"
          minLength={MIN_PASSWORD_LENGTH}
        />

        <Button type="submit" loading={loading} className="mt-2 w-full">
          {t("setup.submit", "Create account")}
        </Button>
      </form>
    </AuthLayout>
  )
}
