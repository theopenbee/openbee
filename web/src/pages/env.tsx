import { useTranslation } from "react-i18next"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { EnvConfigPanel } from "@/components/env-config-panel"
import { DEFAULT_BEE_ID } from "@/lib/types"

export function Settings() {
  const { t } = useTranslation()

  return (
    <FadeIn>
      <div className="w-full max-w-form">
        <PageHeader
          title={t("envConfig.title", "Environment variables")}
          subtitle={t(
            "envConfig.pageDescription",
            "Passed to the Bee and workers when they start. Values are encrypted and stay hidden after you save them.",
          )}
        />
        <div className="grid gap-10">
          <EnvConfigPanel
            scope="global"
            title={t("envConfig.globalTitle", "Global variables")}
            description={t("envConfig.globalDescription", "Available to the Bee and every worker.")}
          />
          <EnvConfigPanel
            scope="bee"
            scopeId={DEFAULT_BEE_ID}
            title={t("envConfig.beeTitle", "Bee variables")}
            description={t(
              "envConfig.beeDescription",
              "Only for the Bee coordinator. A key set here overrides the global value.",
            )}
          />
        </div>
      </div>
    </FadeIn>
  )
}
