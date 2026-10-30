import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { Select } from "@/components/arc/select/select"
import { engineSelectOptions, type EngineSelectOption } from "@/lib/engines"
import type { EngineChoice } from "@/lib/types"

export function useEngineOptions(engines: readonly EngineChoice[], stored?: string): EngineSelectOption[] {
  const { t } = useTranslation()
  return useMemo(
    () => engineSelectOptions(engines, stored, (id) => t("workers.form.engineUnavailable", { engine: id })),
    [engines, stored, t],
  )
}

interface EngineSelectProps {
  id?: string
  label: string
  description?: string
  placeholder?: string
  engines: readonly EngineChoice[]
  value: string
  onValueChange: (value: string) => void
  disabled?: boolean
}

export function EngineSelect({ engines, value, description, disabled, onValueChange, ...rest }: EngineSelectProps) {
  const { t } = useTranslation()
  const options = useEngineOptions(engines, value)
  const empty = options.length === 0

  return (
    <Select
      {...rest}
      description={empty ? t("workers.form.noEngine") : description}
      options={options}
      value={value}
      onValueChange={(next) => next && onValueChange(next)}
      disabled={disabled || empty}
    />
  )
}
