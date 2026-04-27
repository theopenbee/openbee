import { useEffect } from "react"
import { useTranslation } from "react-i18next"
import { Shuffle, Loader2 } from "lucide-react"
import { useRandomWorkerName } from "@/hooks/use-workers"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { getErrorMessage } from "@/lib/utils"

interface WorkerNameFieldProps {
  id: string
  open: boolean
  value: string
  onChange: (value: string) => void
  onError: (message: string) => void
  autoFocus?: boolean
}

export function WorkerNameField({ id, open, value, onChange, onError, autoFocus }: WorkerNameFieldProps) {
  const { t } = useTranslation()
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

  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>
        {t("workers.form.name")}
        <span className="ml-1 text-destructive" aria-hidden>*</span>
      </Label>
      <div className="flex gap-2">
        <Input
          id={id}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={t("workers.form.namePlaceholder")}
          required
          autoFocus={autoFocus}
          className="flex-1"
        />
        <Tooltip open={nameExhausted || undefined}>
          <TooltipTrigger render={<span className="inline-flex" />}>
            <Button
              type="button"
              variant="outline"
              size="icon"
              disabled={nameExhausted || randomName.isPending}
              onClick={handleRandomName}
              aria-label={t("workers.form.randomName")}
            >
              {randomName.isPending
                ? <Loader2 className="size-4 animate-spin" />
                : <Shuffle className="size-4" />
              }
            </Button>
          </TooltipTrigger>
          {nameExhausted && (
            <TooltipContent>
              <p>{t("workers.form.randomNameExhausted")}</p>
            </TooltipContent>
          )}
        </Tooltip>
      </div>
      <p className="text-xs text-muted-foreground">{t("workers.form.nameHelper")}</p>
    </div>
  )
}
