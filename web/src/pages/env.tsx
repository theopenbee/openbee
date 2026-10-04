import { useTranslation } from "react-i18next"
import { FadeIn } from "@/components/fade-in"
import { PageHeader } from "@/components/page-header"
import { EnvConfigPanel } from "@/components/env-config-panel"
import { DEFAULT_BEE_ID } from "@/lib/types"

// Env variables: one section per scope. Each EnvConfigPanel carries its own
// heading row (title + Add action) above a white table surface, so the
// sections sit directly on the canvas rather than inside another card.
export function Settings() {
  const { t } = useTranslation()

  return (
    <FadeIn>
      <div className="w-full">
        <PageHeader title={t("nav.settings")} />

        <div className="space-y-8">
          <EnvConfigPanel
            scope="global"
            title={t("envConfig.globalTitle")}
          />

          <EnvConfigPanel
            scope="bee"
            scopeId={DEFAULT_BEE_ID}
            title={t("envConfig.beeTitle")}
          />
        </div>
      </div>
    </FadeIn>
  )
}
