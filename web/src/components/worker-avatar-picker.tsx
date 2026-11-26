import { useId, useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { ShuffleIcon } from "lucide-react"
import { Alert } from "@/components/arc/alert/alert"
import { Button } from "@/components/arc/button/button"
import { Dialog, DialogContent } from "@/components/arc/dialog/dialog"
import { WorkerAvatar } from "@/components/worker-avatar"
import { useAvatars } from "@/hooks/use-avatars"
import { useWorkers } from "@/hooks/use-workers"
import { avatarLabel, pickRandomAvatar } from "@/lib/avatars"
import { localize } from "@/lib/plugins"
import type { AvatarPack } from "@/lib/types"
import { cn } from "@/lib/utils"

interface WorkerAvatarFieldProps {
  workerId: string
  name: string
  value: string
  onChange: (avatar: string) => void
  disabled?: boolean
}

export function WorkerAvatarField({ workerId, name, value, onChange, disabled }: WorkerAvatarFieldProps) {
  const { t } = useTranslation()
  const labelId = useId()
  const avatars = useAvatars()
  const workers = useWorkers()
  const [open, setOpen] = useState(false)
  const packs = useMemo(() => (avatars.data ?? []).filter((pack) => pack.avatars.length > 0), [avatars.data])
  const used = useMemo(
    () => new Set((workers.data ?? []).filter((w) => w.id !== workerId && w.avatar).map((w) => w.avatar as string)),
    [workers.data, workerId],
  )

  const select = (avatar: string) => {
    onChange(avatar)
    setOpen(false)
  }

  return (
    <div className="grid gap-2" role="group" aria-labelledby={labelId}>
      <span id={labelId} className="text-sm font-medium text-foreground">
        {t("workers.form.avatar")}
      </span>
      <div className="flex min-w-0 items-center gap-4">
        <WorkerAvatar workerId={workerId} name={name} avatar={value} size="lg" />
        {avatars.isError ? (
          <p className="text-sm text-danger">{t("workers.form.avatarLoadError")}</p>
        ) : packs.length > 0 ? (
          <Button type="button" variant="secondary" size="sm" onClick={() => setOpen(true)} disabled={disabled}>
            {t("workers.form.avatarChange")}
          </Button>
        ) : (
          avatars.isSuccess && <p className="text-sm text-secondary">{t("workers.form.avatarNone")}</p>
        )}
      </div>
      <AvatarPickerDialog
        open={open}
        onOpenChange={setOpen}
        workerId={workerId}
        name={name}
        value={value}
        packs={packs}
        onSelect={select}
        onRandom={() => select(pickRandomAvatar(packs, used))}
      />
    </div>
  )
}

interface AvatarPickerDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  workerId: string
  name: string
  value: string
  packs: readonly AvatarPack[]
  onSelect: (avatar: string) => void
  onRandom: () => void
}

function AvatarPickerDialog({ open, onOpenChange, workerId, name, value, packs, onSelect, onRandom }: AvatarPickerDialogProps) {
  const { t, i18n } = useTranslation()
  const named = packs.length > 1
  const tile = (avatar: string, label: string) => (
    <AvatarTile key={avatar || "initials"} workerId={workerId} name={name} avatar={avatar} label={label} selected={value === avatar} onSelect={onSelect} />
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title={t("workers.form.avatarDialogTitle")} description={t("workers.form.avatarDialogDescription", { name })}>
        {packs.length === 0 ? (
          <Alert tone="info" title={t("workers.form.avatarNone")} />
        ) : (
          <div className="grid gap-4">
            <div className="flex justify-end">
              <Button type="button" variant="secondary" size="sm" onClick={onRandom}>
                <ShuffleIcon size={16} strokeWidth={1.75} aria-hidden="true" />
                {t("workers.form.avatarRandom")}
              </Button>
            </div>
            <div className="-mx-2 grid max-h-[min(56vh,26rem)] gap-4 overflow-y-auto px-2 py-1">
              <div className={gridClass}>{tile("", t("workers.form.avatarInitials"))}</div>
              {packs.map((pack) => (
                <section key={pack.id} className="grid gap-2" aria-label={localize(pack.name, i18n.language) || pack.id}>
                  {named && <h3 className="text-xs font-medium text-secondary">{localize(pack.name, i18n.language) || pack.id}</h3>}
                  <div className={gridClass}>
                    {pack.avatars.map((avatar) => tile(avatar, t("workers.form.avatarOption", { name: avatarLabel(avatar) })))}
                  </div>
                </section>
              ))}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

const gridClass = "grid grid-cols-[repeat(auto-fill,minmax(3.25rem,1fr))] gap-2"

interface AvatarTileProps {
  workerId: string
  name: string
  avatar: string
  label: string
  selected: boolean
  onSelect: (avatar: string) => void
}

function AvatarTile({ workerId, name, avatar, label, selected, onSelect }: AvatarTileProps) {
  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={selected}
      title={label}
      onClick={() => onSelect(avatar)}
      className={cn(
        "grid place-items-center justify-self-center rounded-full p-0.5 outline-none ring-2 ring-transparent ring-offset-2 ring-offset-surface-raised transition-shadow hover:ring-border-strong focus-visible:ring-accent motion-reduce:transition-none",
        selected && "ring-accent hover:ring-accent",
      )}
    >
      <WorkerAvatar workerId={workerId} name={name} avatar={avatar} size="lg" />
    </button>
  )
}
