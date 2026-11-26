import { useEffect, useId, useMemo, useRef, useState } from "react"
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
        onRandom={() => select(pickRandomAvatar(packs, new Set([...used, value])))}
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
  const tiles = (avatars: readonly string[]) =>
    avatars.map((avatar) => (
      <AvatarTile
        key={avatar}
        workerId={workerId}
        name={name}
        avatar={avatar}
        label={t("workers.form.avatarOption", { name: avatarLabel(avatar) })}
        selected={value === avatar}
        onSelect={onSelect}
      />
    ))
  const choices = (
    <>
      <AvatarTile workerId={workerId} name={name} avatar="" label={t("workers.form.avatarInitials")} selected={value === ""} onSelect={onSelect} />
      <button type="button" aria-label={t("workers.form.avatarRandom")} title={t("workers.form.avatarRandom")} onClick={onRandom} className={tileClass}>
        <span className="grid size-12 place-items-center rounded-full border border-dashed border-border-strong text-secondary">
          <ShuffleIcon size={18} strokeWidth={1.75} aria-hidden="true" />
        </span>
      </button>
    </>
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title={t("workers.form.avatarDialogTitle")} description={t("workers.form.avatarDialogDescription", { name })}>
        {packs.length === 0 ? (
          <Alert tone="info" title={t("workers.form.avatarNone")} />
        ) : (
          <div className="-mx-2 grid max-h-[min(60vh,28rem)] gap-4 overflow-y-auto px-2 py-1">
            {packs.length === 1 ? (
              <div className={gridClass}>
                {choices}
                {tiles(packs[0].avatars)}
              </div>
            ) : (
              <>
                <div className={gridClass}>{choices}</div>
                {packs.map((pack) => {
                  const packName = localize(pack.name, i18n.language) || pack.id
                  return (
                    <section key={pack.id} className="grid gap-2" aria-label={packName}>
                      <h3 className="text-xs font-medium text-secondary">{packName}</h3>
                      <div className={gridClass}>{tiles(pack.avatars)}</div>
                    </section>
                  )
                })}
              </>
            )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

const gridClass = "grid grid-cols-[repeat(auto-fill,minmax(3.25rem,1fr))] gap-2"

const tileClass =
  "grid place-items-center justify-self-center rounded-full p-0.5 outline-none ring-2 ring-transparent ring-offset-2 ring-offset-surface-raised transition-shadow hover:ring-border-strong focus-visible:ring-accent motion-reduce:transition-none"

interface AvatarTileProps {
  workerId: string
  name: string
  avatar: string
  label: string
  selected: boolean
  onSelect: (avatar: string) => void
}

function AvatarTile({ workerId, name, avatar, label, selected, onSelect }: AvatarTileProps) {
  const ref = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (selected) ref.current?.scrollIntoView({ block: "center" })
  }, [selected])

  return (
    <button
      ref={ref}
      type="button"
      aria-label={label}
      aria-pressed={selected}
      title={label}
      onClick={() => onSelect(avatar)}
      className={cn(tileClass, selected && "ring-accent hover:ring-accent")}
    >
      <WorkerAvatar workerId={workerId} name={name} avatar={avatar} size="lg" />
    </button>
  )
}
