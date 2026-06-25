import { useTranslation } from "react-i18next"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { DetailSection } from "@/components/detail-primitives"
import { EnvConfigPanel } from "@/components/env-config-panel"
import { DEFAULT_BEE_ID } from "@/lib/types"

export function Settings() {
  const { t } = useTranslation()

  return (
    <FadeIn>
      <div className="mx-auto w-full max-w-3xl space-y-6">
        <PageHeader title={t("nav.settings")} />

        <DetailSection className="p-5 sm:p-6">
          <EnvConfigPanel
            scope="global"
            title={t("envConfig.globalTitle")}
            description={t("envConfig.globalHint")}
          />
        </DetailSection>

        <DetailSection className="p-5 sm:p-6">
          <EnvConfigPanel
            scope="bee"
            scopeId={DEFAULT_BEE_ID}
            title={t("envConfig.beeTitle")}
            description={t("envConfig.beeHint")}
          />
        </DetailSection>
      </div>
    </FadeIn>
  )
}
