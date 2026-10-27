import { useState } from "react"
import { Link, useSearchParams } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ChevronLeftIcon, PanelRightCloseIcon, PanelRightOpenIcon, SquareIcon, SquarePenIcon } from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Avatar } from "@/components/arc/avatar/avatar"
import { Badge } from "@/components/arc/badge/badge"
import { Button } from "@/components/arc/button/button"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { Tooltip } from "@/components/arc/tooltip/tooltip"
import { BeeAvatar } from "@/components/chat/bee-avatar"
import { WorkerAvatar } from "@/components/worker-avatar"
import { useClearConversation, useStopConversation } from "@/hooks/use-conversations"
import { activeTasksOf, taskTypeLabelKey } from "@/lib/chat-view"
import { BEE_CONVERSATION } from "@/lib/conversations"
import { panelFromSearch, panelToSearch } from "@/lib/routes"
import type { ActiveTask, ConversationId, Worker } from "@/lib/types"
import { getErrorMessage } from "@/lib/utils"

interface ChatHeaderProps {
  conversation: ConversationId
  worker: Worker | undefined
  name: string
  status: string
  busy: boolean
}

const HEADER = "flex h-16 shrink-0 items-center gap-3 border-b border-border px-3 md:px-5"

export function ChatBackLink() {
  const { t } = useTranslation()
  return (
    <Link
      to="/chat"
      aria-label={t("chat.back")}
      className="grid size-10 shrink-0 place-items-center rounded-control text-secondary transition-colors hover:bg-surface-muted hover:text-foreground md:hidden"
    >
      <ChevronLeftIcon size={20} strokeWidth={1.75} aria-hidden="true" />
    </Link>
  )
}

export function ChatHeaderPlaceholder() {
  return (
    <header className={HEADER}>
      <ChatBackLink />
    </header>
  )
}

export function ChatHeader({ conversation, worker, name, status, busy }: ChatHeaderProps) {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const isBee = conversation === BEE_CONVERSATION
  const panelOpen = panelFromSearch(searchParams)?.mode === "worker"
  const panelLabel = t(panelOpen ? "chat.closePanel" : "chat.openPanel")

  const togglePanel = () =>
    setSearchParams((current) =>
      panelToSearch(panelOpen ? null : { mode: "worker", section: "overview" }, current),
    )

  return (
    <header className={HEADER}>
      <ChatBackLink />
      {isBee ? (
        <BeeAvatar />
      ) : worker ? (
        <WorkerAvatar workerId={worker.id} name={worker.name} status={worker.status} />
      ) : (
        <Avatar name="?" aria-hidden="true" />
      )}
      <div className="grid min-w-0 flex-1 justify-items-start">
        <h1 className="flex max-w-full min-w-0">
          {isBee ? (
            <span className="truncate text-base font-medium text-foreground">{name}</span>
          ) : (
            <button
              type="button"
              onClick={togglePanel}
              aria-expanded={panelOpen}
              className="-mx-2 min-w-0 truncate rounded-control px-2 text-left text-base font-medium text-foreground transition-colors hover:bg-surface-muted"
            >
              {name}
            </button>
          )}
        </h1>
        <p className="max-w-full truncate text-xs text-secondary">{status}</p>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        {!isBee && busy && <StopButton conversation={conversation} />}
        <NewConversationButton conversation={conversation} isBee={isBee} />
        {!isBee && (
          <Tooltip content={panelLabel}>
            <Button variant="ghost" size="sm" aria-label={panelLabel} aria-pressed={panelOpen} onClick={togglePanel}>
              {panelOpen ? (
                <PanelRightCloseIcon size={18} strokeWidth={1.75} aria-hidden="true" />
              ) : (
                <PanelRightOpenIcon size={18} strokeWidth={1.75} aria-hidden="true" />
              )}
            </Button>
          </Tooltip>
        )}
      </div>
    </header>
  )
}

function StopButton({ conversation }: { conversation: ConversationId }) {
  const { t } = useTranslation()
  const stop = useStopConversation(conversation)

  const handleStop = () =>
    stop.mutate(undefined, {
      onError: (error) => toast.error(t("chat.stopFailed"), { description: getErrorMessage(error) }),
    })

  return (
    <Button variant="secondary" size="sm" loading={stop.isPending} onClick={handleStop}>
      <SquareIcon size={14} strokeWidth={1.75} aria-hidden="true" />
      {t("chat.stop")}
    </Button>
  )
}

function NewConversationButton({ conversation, isBee }: { conversation: ConversationId; isBee: boolean }) {
  const { t } = useTranslation()
  const clear = useClearConversation(conversation)
  const [open, setOpen] = useState(false)
  const [activeTasks, setActiveTasks] = useState<ActiveTask[] | null>(null)
  const label = t("chat.newConversation")
  const failed = clear.error !== null && activeTasksOf(clear.error) === null

  const show = () => {
    clear.reset()
    setActiveTasks(null)
    setOpen(true)
  }

  const run = async (force: boolean) => {
    try {
      await clear.mutateAsync(force)
      setOpen(false)
    } catch (error) {
      const tasks = activeTasksOf(error)
      if (tasks) setActiveTasks(tasks)
    }
  }

  const description = activeTasks
    ? t("chat.newConversationActive")
    : t(isBee ? "chat.newConversationConfirmBee" : "chat.newConversationConfirm")

  return (
    <>
      <Tooltip content={label}>
        <Button variant="ghost" size="sm" aria-label={label} onClick={show}>
          <SquarePenIcon size={18} strokeWidth={1.75} aria-hidden="true" />
        </Button>
      </Tooltip>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (!clear.isPending) setOpen(next)
        }}
      >
        <DialogContent title={label} description={description}>
          <div className="grid gap-4">
            {activeTasks && activeTasks.length > 0 && <ActiveTaskList tasks={activeTasks} />}
            {failed && (
              <Alert tone="danger" title={t("chat.newConversationFailed")}>
                {getErrorMessage(clear.error)}
              </Alert>
            )}
            <div className="flex justify-end gap-2">
              <Button variant="secondary" onClick={() => setOpen(false)} disabled={clear.isPending}>
                {t("common.cancel")}
              </Button>
              {activeTasks ? (
                <Button variant="danger" loading={clear.isPending} onClick={() => void run(true)}>
                  {t("chat.stopAndRestart")}
                </Button>
              ) : (
                <Button loading={clear.isPending} onClick={() => void run(false)}>
                  {label}
                </Button>
              )}
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}

function ActiveTaskList({ tasks }: { tasks: ActiveTask[] }) {
  const { t } = useTranslation()
  return (
    <ul className="grid max-h-64 gap-2 overflow-y-auto">
      {tasks.map((task) => {
        const typeKey = taskTypeLabelKey(task.type)
        return (
          <li key={task.id} className="grid justify-items-start gap-1.5 rounded-inset border border-border bg-surface-muted px-3 py-2.5">
            <p className="line-clamp-2 text-sm text-foreground">{task.instruction}</p>
            {typeKey && (
              <Badge tone="neutral" size="sm">
                {t(typeKey)}
              </Badge>
            )}
          </li>
        )
      })}
    </ul>
  )
}
