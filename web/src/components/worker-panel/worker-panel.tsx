import { useState } from "react"
import { useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import { EllipsisIcon, PencilIcon, Trash2Icon } from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { DropdownMenu } from "@/components/arc/dropdown-menu/dropdown-menu"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/arc/tabs/tabs"
import { EngineIcon } from "@/components/agent-icons/engine-icon"
import { StatusBadge } from "@/components/status-badge"
import { WorkerAvatar } from "@/components/worker-avatar"
import { WorkerConstraintsPanel } from "@/components/worker-constraints-panel"
import { DeleteWorkerDialog, type DeleteTarget } from "@/components/worker-panel/delete-worker-dialog"
import { WorkerEnvSection } from "@/components/worker-panel/worker-env-section"
import { WorkerInfoForm } from "@/components/worker-panel/worker-info-form"
import { WorkerPermissions } from "@/components/worker-panel/worker-permissions"
import { useWorker } from "@/hooks/use-workers"
import { BEE_CONVERSATION, conversationPath } from "@/lib/conversations"
import { formatEngineLabel } from "@/lib/format"
import { DEFAULT_PANEL_SECTION, isPanelSection, type PanelSection, type PanelState } from "@/lib/routes"
import type { Worker } from "@/lib/types"
import { getErrorMessage } from "@/lib/utils"

const SECTIONS: { key: PanelSection; labelKey: string }[] = [
  { key: "constraints", labelKey: "chat.panelTabs.constraints" },
  { key: "permissions", labelKey: "chat.panelTabs.permissions" },
  { key: "env", labelKey: "chat.panelTabs.env" },
]

const ICON = { size: 16, strokeWidth: 1.75, "aria-hidden": true } as const

interface WorkerPanelProps {
  workerId: string
  panel: PanelState
  onPanelChange: (panel: PanelState) => void
}

export function WorkerPanel({ workerId, panel, onPanelChange }: WorkerPanelProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: worker, error, refetch } = useWorker(workerId)
  const [editing, setEditing] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)
  const section = panel?.mode === "worker" ? panel.section : DEFAULT_PANEL_SECTION

  if (error && !worker) {
    return (
      <Alert tone="danger" title={t("workerDetail.loadError")}>
        <span className="grid justify-items-start gap-3">
          <span>{getErrorMessage(error)}</span>
          <Button variant="secondary" size="sm" onClick={() => refetch()}>
            {t("common.retry")}
          </Button>
        </span>
      </Alert>
    )
  }

  if (!worker) return <WorkerPanelSkeleton />

  const handleDeleted = () => {
    setDeleteTarget(null)
    navigate(conversationPath(BEE_CONVERSATION), { replace: true })
  }

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-6">
      <WorkerHeader
        worker={worker}
        editing={editing}
        onEdit={() => setEditing(true)}
        onDelete={() => setDeleteTarget({ id: worker.id, name: worker.name, work_dir: worker.work_dir })}
      />

      {editing ? (
        <WorkerInfoForm worker={worker} onDone={() => setEditing(false)} />
      ) : (
        <Tabs
          value={section}
          onValueChange={(value) => {
            if (isPanelSection(value)) onPanelChange({ mode: "worker", section: value })
          }}
        >
          <TabsList aria-label={t("workerDetail.sectionNav")}>
            {SECTIONS.map(({ key, labelKey }) => (
              <TabsTrigger key={key} value={key} className="min-w-0! px-2!">
                {t(labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="constraints">
            <WorkerConstraintsPanel worker={worker} />
          </TabsContent>
          <TabsContent value="permissions">
            <WorkerPermissions worker={worker} />
          </TabsContent>
          <TabsContent value="env">
            <WorkerEnvSection worker={worker} />
          </TabsContent>
        </Tabs>
      )}

      <DeleteWorkerDialog target={deleteTarget} onClose={() => setDeleteTarget(null)} onDeleted={handleDeleted} />
    </div>
  )
}

interface WorkerHeaderProps {
  worker: Worker
  editing: boolean
  onEdit: () => void
  onDelete: () => void
}

function WorkerHeader({ worker, editing, onEdit, onDelete }: WorkerHeaderProps) {
  const { t } = useTranslation()

  return (
    <div className="grid gap-4">
      <div className="flex min-w-0 items-center gap-4">
        <WorkerAvatar workerId={worker.id} name={worker.name} status={worker.status} size="lg" />
        <div className="grid min-w-0 flex-1 gap-1.5">
          <h3 className="truncate text-lg font-medium text-foreground">{worker.name}</h3>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-secondary">
            <StatusBadge status={worker.status} />
            {worker.engine && (
              <span className="inline-flex min-w-0 items-center gap-1.5">
                <EngineIcon engine={worker.engine} className="size-3.5 shrink-0" />
                <span className="truncate">{formatEngineLabel(worker.engine, t)}</span>
              </span>
            )}
          </div>
        </div>
      </div>
      <p className="text-sm whitespace-pre-wrap text-secondary wrap-anywhere">
        {worker.description || t("common.noDescription")}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="secondary" size="sm" onClick={onEdit} disabled={editing}>
          <PencilIcon {...ICON} />
          {t("workerDetail.editInfo")}
        </Button>
        <DropdownMenu
          iconOnly
          icon={<EllipsisIcon size={16} strokeWidth={1.75} />}
          label={t("workers.rowActions", { name: worker.name })}
          items={[
            {
              label: t("workers.deleteWorker"),
              icon: <Trash2Icon size={16} strokeWidth={1.75} />,
              destructive: true,
              onSelect: onDelete,
            },
          ]}
        />
      </div>
    </div>
  )
}

function WorkerPanelSkeleton() {
  return (
    <div className="grid gap-6" aria-busy="true">
      <div className="flex items-center gap-4">
        <div className="skeleton size-12 shrink-0 rounded-full" />
        <div className="grid flex-1 gap-2">
          <div className="skeleton h-5 w-32" />
          <div className="skeleton h-4 w-40" />
        </div>
      </div>
      <div className="skeleton h-4 w-full" />
      <div className="skeleton h-10 w-64 rounded-control" />
      <div className="grid gap-3">
        <div className="skeleton h-24 w-full rounded-inset" />
        <div className="skeleton h-40 w-full rounded-inset" />
      </div>
    </div>
  )
}
