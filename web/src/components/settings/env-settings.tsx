import { useTranslation } from "react-i18next"
import { FadeIn } from "@/components/fade-in"
import { EnvConfigPanel } from "@/components/env-config-panel"
import { DEFAULT_BEE_ID } from "@/lib/types"

export function EnvSettings() {
  const { t } = useTranslation()

  return (
    <FadeIn className="grid gap-10">
      <EnvConfigPanel
        scope="global"
        title={t("envConfig.globalTitle", "Global variables")}
        description={t("envConfig.globalDescription", "Available to the Bee and every worker.")}
        inset
      />
      <EnvConfigPanel
        scope="bee"
        scopeId={DEFAULT_BEE_ID}
        title={t("envConfig.beeTitle", "Bee variables")}
        description={t(
          "envConfig.beeDescription",
          "Only for the Bee coordinator. A key set here overrides the global value.",
        )}
        inset
      />
    </FadeIn>
  )
}
