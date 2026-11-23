import { Streamdown, type ControlsConfig, type LinkSafetyConfig } from "streamdown"
import { code } from "@streamdown/code"
import { LinkSafetyDialog } from "@/components/link-safety-dialog"
import { CHAT_REMARK_PLUGINS } from "@/lib/markdown"
import { cn } from "@/lib/utils"
import styles from "./chat-markdown.module.css"

const STREAMDOWN_PLUGINS = { code }

const LINK_SAFETY: LinkSafetyConfig = {
  enabled: true,
  renderModal: (props) => <LinkSafetyDialog {...props} />,
}

export function ChatMarkdown({
  children,
  breaks = false,
  controls,
  className,
}: {
  children: string
  breaks?: boolean
  controls?: ControlsConfig
  className?: string
}) {
  return (
    <div className={cn(styles.root, className)}>
      <Streamdown
        mode="static"
        plugins={STREAMDOWN_PLUGINS}
        remarkPlugins={breaks ? CHAT_REMARK_PLUGINS : undefined}
        controls={controls}
        linkSafety={LINK_SAFETY}
      >
        {children}
      </Streamdown>
    </div>
  )
}
