import { useTranslation } from "react-i18next"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"

interface LoadErrorAlertProps {
  title: string
  error: Error
  retrying?: boolean
  onRetry: () => void
}

export function LoadErrorAlert({ title, error, retrying, onRetry }: LoadErrorAlertProps) {
  const { t } = useTranslation()
  return (
    <Alert tone="danger" title={title}>
      <span className="block break-words">{error.message}</span>
      <span className="mt-3 flex">
        <Button variant="secondary" size="sm" loading={retrying} onClick={onRetry}>
          {t("common.retry")}
        </Button>
      </span>
    </Alert>
  )
}
