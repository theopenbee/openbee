import { forwardRef, useId } from "react"
import { PasswordField, type PasswordFieldProps } from "@/components/arc/password-field/password-field"

export interface AuthPasswordFieldProps extends PasswordFieldProps {
  error?: string
}

export const AuthPasswordField = forwardRef<HTMLInputElement, AuthPasswordFieldProps>(function AuthPasswordField(
  { error, id, ...props },
  ref,
) {
  const generatedId = useId()
  const controlId = id ?? generatedId
  const errorId = `${controlId}-error`
  const describedBy = [props["aria-describedby"], error ? errorId : undefined].filter(Boolean).join(" ") || undefined

  return (
    <div className="grid min-w-0">
      <PasswordField
        {...props}
        ref={ref}
        id={controlId}
        aria-invalid={error ? true : props["aria-invalid"]}
        aria-describedby={describedBy}
      />
      {error ? (
        <span id={errorId} role="alert" className="animate-fade-in block pt-2 text-xs text-danger">
          {error}
        </span>
      ) : null}
    </div>
  )
})
