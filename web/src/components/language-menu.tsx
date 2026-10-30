import { CheckIcon, LanguagesIcon } from "lucide-react"
import { DropdownMenu } from "@/components/arc/dropdown-menu/dropdown-menu"
import { useLanguage } from "@/hooks/use-language"
import { LANGUAGES } from "@/lib/language"

export function LanguageMenu({ className }: { className?: string }) {
  const { language, setLanguage } = useLanguage()
  const current = LANGUAGES.find((option) => option.value === language) ?? LANGUAGES[0]

  return (
    <DropdownMenu
      label={current.label}
      icon={<LanguagesIcon size={16} strokeWidth={1.75} />}
      className={className}
      items={LANGUAGES.map((option) => ({
        label: option.label,
        icon: (
          <CheckIcon
            size={16}
            strokeWidth={1.75}
            className={option.value === language ? undefined : "invisible"}
          />
        ),
        onSelect: () => setLanguage(option.value),
      }))}
    />
  )
}
