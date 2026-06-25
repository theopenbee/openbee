import { useTranslation } from "react-i18next"
import { SelectItem } from "@/components/ui/select"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { formatEngineLabel } from "@/lib/format"
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
        <SelectItem key={e} value={e}>
          <span className="flex items-center gap-2">
            <EngineIcon engine={e} className="size-4" />
            {formatEngineLabel(e, t)}
          </span>
        </SelectItem>
      ))}
    </>
  )
}
