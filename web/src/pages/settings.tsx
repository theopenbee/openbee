import { useTranslation } from "react-i18next"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { DetailSection } from "@/components/detail-primitives"
import { EnvConfigPanel } from "@/components/env-config-panel"

export function Settings() {
  const { t } = useTranslation()

  return (
    <FadeIn>
      <div className="space-y-6">
        <PageHeader title={t("nav.settings")} />

        <DetailSection className="p-5 sm:p-6 space-y-4">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
              {t("envConfig.globalTitle")}
            </p>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {t("envConfig.globalHint")}
            </p>
          </div>
          <EnvConfigPanel scope="global" />
        </DetailSection>
      </div>
    </FadeIn>
  )
}
