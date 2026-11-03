import { Streamdown } from "streamdown"
import { code } from "@streamdown/code"
import { CHAT_REMARK_PLUGINS } from "@/lib/markdown"
import { cn } from "@/lib/utils"
import styles from "./chat-markdown.module.css"

const STREAMDOWN_PLUGINS = { code }

export function ChatMarkdown({
  children,
  breaks = false,
  className,
}: {
  children: string
  breaks?: boolean
  className?: string
}) {
  return (
    <div className={cn(styles.root, className)}>
      <Streamdown
        mode="static"
        plugins={STREAMDOWN_PLUGINS}
        remarkPlugins={breaks ? CHAT_REMARK_PLUGINS : undefined}
      >
        {children}
      </Streamdown>
    </div>
  )
}
