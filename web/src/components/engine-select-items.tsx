import { useTranslation } from "react-i18next"
import { SelectItem } from "@/components/ui/select"
import { ENGINES } from "@/lib/types"
import type { Engine } from "@/lib/types"

interface EngineSelectItemsProps {
  engines?: readonly Engine[]
}

export function EngineSelectItems({ engines = ENGINES }: EngineSelectItemsProps) {
  const { t } = useTranslation()
  return (
    <>
      {engines.map((e) => (
        <SelectItem key={e} value={e}>{t(`workers.engines.${e}`)}</SelectItem>
      ))}
    </>
  )
}
