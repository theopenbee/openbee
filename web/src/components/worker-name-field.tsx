import { useEffect, useId } from "react"
import { useTranslation } from "react-i18next"
import { Shuffle } from "lucide-react"
import { useRandomWorkerName } from "@/hooks/use-workers"
import { Button } from "@/components/arc/button/button"
import { Input } from "@/components/arc/input/input"
import { Tooltip } from "@/components/arc/tooltip/tooltip"
import { getErrorMessage } from "@/lib/utils"

interface WorkerNameFieldProps {
  id: string
  open: boolean
  value: string
  onChange: (value: string) => void
  onError: (message: string) => void
  autoFocus?: boolean
  disabled?: boolean
}

export function WorkerNameField({ id, open, value, onChange, onError, autoFocus, disabled }: WorkerNameFieldProps) {
  const { t } = useTranslation()
  const hintId = useId()
  const randomName = useRandomWorkerName()
  const nameExhausted = randomName.data?.exhausted ?? false

  useEffect(() => {
    if (open) randomName.reset()
  }, [open])

  const handleRandomName = async () => {
    try {
      const result = await randomName.mutateAsync()
      if (!result.exhausted && result.name) {
        onChange(result.name)
      }
    } catch (err) {
      onError(getErrorMessage(err))
    }
  }

  const randomLabel = t("workers.form.randomName")

  return (
    <div className="grid min-w-0">
      <div className="flex items-end gap-2">
        <div className="min-w-0 flex-1">
          <Input
            id={id}
            label={t("workers.form.name")}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder={t("workers.form.namePlaceholder")}
            required
            autoComplete="off"
            autoFocus={autoFocus}
            disabled={disabled}
            aria-describedby={hintId}
          />
        </div>
        <Tooltip content={randomLabel}>
          <Button
            type="button"
            variant="secondary"
            disabled={nameExhausted || disabled}
            loading={randomName.isPending}
            onClick={handleRandomName}
            aria-label={randomLabel}
            className="shrink-0"
          >
            <Shuffle size={16} strokeWidth={1.75} aria-hidden="true" />
          </Button>
        </Tooltip>
      </div>
      <p id={hintId} className="pt-2 text-xs text-muted" aria-live="polite">
        {nameExhausted ? t("workers.form.randomNameExhausted") : t("workers.form.nameHelper")}
      </p>
    </div>
  )
}
