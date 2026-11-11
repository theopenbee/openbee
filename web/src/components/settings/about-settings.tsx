import { useTranslation } from "react-i18next"
import { ExternalLinkIcon } from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { CopyButton } from "@/components/copy-button"
import { FadeIn } from "@/components/fade-in"
import { SettingsGroup, SettingsRow, SettingsSkeleton } from "@/components/settings/settings-layout"
import { useServerVersion } from "@/hooks/use-config"

const RELEASES_URL = "https://github.com/theopenbee/openbee/releases"
const UPGRADE_COMMAND = "openbee upgrade"

export function AboutSettings() {
  const { t } = useTranslation()
  const { data, error, isLoading, refetch } = useServerVersion()

  return (
    <FadeIn>
      {error ? (
        <Alert tone="danger" title={t("about.loadError")}>
          <span className="grid justify-items-start gap-3">
            <span>{error.message}</span>
            <Button variant="secondary" size="sm" onClick={() => refetch()}>
              {t("common.retry")}
            </Button>
          </span>
        </Alert>
      ) : isLoading || !data ? (
        <SettingsSkeleton />
      ) : (
        <section className="grid gap-4" aria-labelledby="about-server">
          <h2 id="about-server" className="text-base font-medium text-foreground">
            {t("about.server.title")}
          </h2>
          <SettingsGroup>
            <SettingsRow
              title={t("about.version")}
              description={t(data.bundled ? "about.server.bundledDescription" : "about.server.description")}
            >
              <p className="font-mono text-sm text-foreground">{data.version}</p>
            </SettingsRow>
            {!data.bundled && (
              <SettingsRow title={t("about.server.upgrade")} description={t("about.server.upgradeDescription")}>
                <div className="flex min-w-0 items-center justify-between gap-2 rounded-control border border-border bg-surface-muted py-1 pr-1 pl-4">
                  <code className="min-w-0 truncate font-mono text-sm text-foreground">{UPGRADE_COMMAND}</code>
                  <CopyButton value={UPGRADE_COMMAND} label={t("about.server.copyCommand")} />
                </div>
              </SettingsRow>
            )}
            <SettingsRow title={t("about.releases.title")} description={t("about.releases.description")}>
              <a
                href={RELEASES_URL}
                target="_blank"
                rel="noreferrer"
                className="text-sm text-accent-strong wrap-anywhere hover:underline"
              >
                {t("about.releases.link")}
                <ExternalLinkIcon size={14} strokeWidth={1.75} className="ml-1 inline align-text-top" aria-hidden="true" />
              </a>
            </SettingsRow>
          </SettingsGroup>
        </section>
      )}
    </FadeIn>
  )
}
