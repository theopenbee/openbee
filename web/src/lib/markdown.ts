import { defaultRemarkPlugins } from "streamdown"
import remarkBreaks from "remark-breaks"

export const CHAT_REMARK_PLUGINS = [...Object.values(defaultRemarkPlugins), remarkBreaks]
