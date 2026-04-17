import { useTranslation } from "react-i18next"
import { SelectItem } from "@/components/ui/select"
import { ENGINES } from "@/lib/types"

export function EngineSelectItems() {
  const { t } = useTranslation()
  return (
    <>
      {ENGINES.map((e) => (
        <SelectItem key={e} value={e}>{t(`workers.engines.${e}`)}</SelectItem>
      ))}
    </>
  )
}
