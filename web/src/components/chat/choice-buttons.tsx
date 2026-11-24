import { useState } from "react"
import { useTranslation } from "react-i18next"
import { CheckIcon } from "lucide-react"
import { Button } from "@/components/arc/button/button"
import { choiceStateOf } from "@/lib/chat-view"
import type { ChatButton } from "@/lib/types"
import { cn } from "@/lib/utils"
import styles from "./choice-buttons.module.css"

interface ChoiceButtonsProps {
  buttons: ChatButton[]
  choice?: number
  onChoose: (index: number) => Promise<void>
}

export function ChoiceButtons({ buttons, choice, onChoose }: ChoiceButtonsProps) {
  const { t } = useTranslation()
  const [pending, setPending] = useState<number | null>(null)

  const choose = async (index: number) => {
    if (pending !== null || typeof choice === "number") return
    setPending(index)
    try {
      await onChoose(index)
    } finally {
      setPending(null)
    }
  }

  return (
    <div className={cn(styles.row, "@container mt-2 flex w-full flex-wrap gap-2")}>
      {buttons.map((button, index) => {
        const state = choiceStateOf(index, choice, pending)
        const danger = button.style === "danger"
        return (
          <Button
            key={index}
            type="button"
            variant={danger ? "danger" : "secondary"}
            size="sm"
            loading={state === "loading"}
            disabled={state === "chosen" || state === "disabled"}
            aria-label={state === "chosen" ? `${button.label}, ${t("chat.choiceChosen")}` : undefined}
            className={cn(state === "chosen" && styles.chosen, state === "chosen" && danger && styles.danger)}
            onClick={() => void choose(index)}
          >
            {state === "chosen" && <CheckIcon size={14} strokeWidth={2} aria-hidden="true" />}
            <span className={styles.label}>{button.label}</span>
          </Button>
        )
      })}
    </div>
  )
}
