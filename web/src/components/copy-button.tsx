import { useTranslation } from "react-i18next"
import { CopyButton as ArcCopyButton } from "@/components/arc/copy-button/copy-button"

export function CopyButton({
  value,
  label,
  className,
}: {
  value: string
  label?: string
  className?: string
}) {
  const { t } = useTranslation()
  return (
    <ArcCopyButton
      value={value}
      label={label ?? t("common.copy")}
      iconOnly
      variant="plain"
      className={className}
    />
  )
}
