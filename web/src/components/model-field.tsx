import { useEffect, useMemo, useReducer, useRef, useState, type KeyboardEvent } from "react"
import { useTranslation } from "react-i18next"
import { Check } from "lucide-react"
import { Input } from "@/components/arc/input/input"
import { TextShimmer } from "@/components/arc/text-shimmer/text-shimmer"
import { useEngineModels } from "@/hooks/use-engine-models"
import {
  CLOSED_SUGGESTIONS,
  highlightedModel,
  modelListNotice,
  suggestionReducer,
  visibleModels,
  type ModelListNotice,
  type ModelListStatus,
} from "@/lib/engine-models"
import type { EngineChoice, EngineModel } from "@/lib/types"
import { cn, isImeComposing } from "@/lib/utils"

export type ModelNotices = Record<ModelListNotice, string>

interface ModelSuggestionsProps {
  id: string
  label: string
  models: readonly EngineModel[]
  active: number
  selected: string
  onPick: (model: EngineModel) => void
  onHover: (index: number) => void
}

export function ModelSuggestions({ id, label, models, active, selected, onPick, onHover }: ModelSuggestionsProps) {
  const rootRef = useRef<HTMLDivElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    rootRef.current?.scrollIntoView?.({ block: "nearest" })
  }, [models.length])

  useEffect(() => {
    if (active < 0) return
    listRef.current?.querySelector(`[data-index="${active}"]`)?.scrollIntoView({ block: "nearest" })
  }, [active])

  return (
    <div
      ref={rootRef}
      className="absolute inset-x-0 top-2 z-50 animate-fade-in overflow-hidden rounded-panel border border-border bg-surface-raised p-2 shadow-floating"
      onMouseDown={(event) => event.preventDefault()}
    >
      <div ref={listRef} id={id} role="listbox" aria-label={label} className="max-h-72 overflow-y-auto overscroll-contain">
        {models.map((model, index) => (
          <div
            key={model.id}
            id={`${id}-${index}`}
            data-index={index}
            role="option"
            aria-selected={index === active}
            className={cn(
              "flex min-h-9 cursor-pointer items-center gap-3 rounded-control px-3 text-sm text-foreground transition-colors motion-reduce:transition-none",
              index === active && "bg-surface-muted",
            )}
            onMouseEnter={() => onHover(index)}
            onClick={() => onPick(model)}
          >
            <span className="min-w-0 flex-1 truncate">{model.id}</span>
            {model.name && <span className="max-w-[45%] shrink-0 truncate text-xs text-muted">{model.name}</span>}
            {model.id === selected && (
              <Check size={16} strokeWidth={1.75} aria-hidden="true" className="shrink-0 text-foreground" />
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

interface ModelHintProps {
  id?: string
  description?: string
  notice: ModelListNotice | null
  notices: ModelNotices
  hidden?: boolean
}

export function ModelHint({ id, description, notice, notices, hidden }: ModelHintProps) {
  if (!notice && !description) return null
  return (
    <p id={id} role={notice ? "status" : undefined} className={cn("pt-2 text-xs text-muted", hidden && "invisible")}>
      {notice === "loading" ? <TextShimmer>{notices.loading}</TextShimmer> : notice ? notices[notice] : description}
    </p>
  )
}

interface ModelComboboxProps {
  id: string
  label: string
  description?: string
  placeholder?: string
  value: string
  onChange: (value: string) => void
  models: readonly EngineModel[]
  status: ModelListStatus
  notices: ModelNotices
  onFocus?: () => void
  disabled?: boolean
}

export function ModelCombobox({
  id,
  label,
  description,
  placeholder,
  value,
  onChange,
  models,
  status,
  notices,
  onFocus,
  disabled,
}: ModelComboboxProps) {
  const listId = `${id}-listbox`
  const hintId = description ? `${id}-hint` : undefined
  const [state, dispatch] = useReducer(suggestionReducer, CLOSED_SUGGESTIONS)
  const visible = visibleModels(models, state)
  const open = state.open && !disabled
  const showPopover = open && visible.length > 0
  const expanded = showPopover
  const notice = open && !showPopover ? modelListNotice(status, models.length, visible.length) : null
  const highlighted = expanded ? highlightedModel(visible, state) : undefined

  const pick = (model: EngineModel) => {
    onChange(model.id)
    dispatch({ type: "close" })
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (isImeComposing(event)) return
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      dispatch({ type: "move", step: event.key === "ArrowDown" ? 1 : -1, count: visible.length })
      return
    }
    if (event.key === "Enter") {
      if (highlighted) {
        event.preventDefault()
        pick(highlighted)
      } else {
        dispatch({ type: "close" })
      }
      return
    }
    if (event.key === "Escape" && showPopover) {
      event.preventDefault()
      event.stopPropagation()
      dispatch({ type: "close" })
    }
  }

  return (
    <div className="grid min-w-0">
      <Input
        id={id}
        label={label}
        value={value}
        placeholder={placeholder}
        role="combobox"
        aria-autocomplete="list"
        aria-expanded={expanded}
        aria-controls={expanded ? listId : undefined}
        aria-activedescendant={highlighted ? `${listId}-${state.active}` : undefined}
        aria-describedby={hintId}
        autoComplete="off"
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        disabled={disabled}
        onFocus={() => {
          onFocus?.()
          dispatch({ type: "open" })
        }}
        onClick={() => dispatch({ type: "open" })}
        onBlur={() => dispatch({ type: "close" })}
        onChange={(event) => {
          onChange(event.target.value)
          dispatch({ type: "input", text: event.target.value })
        }}
        onKeyDown={handleKeyDown}
      />
      <div className="relative">
        {showPopover && (
          <ModelSuggestions
            id={listId}
            label={label}
            models={visible}
            active={state.active}
            selected={value.trim()}
            onPick={pick}
            onHover={(index) => dispatch({ type: "hover", index })}
          />
        )}
      </div>
      <ModelHint id={hintId} description={description} notice={notice} notices={notices} hidden={showPopover} />
    </div>
  )
}

interface ModelFieldProps {
  id: string
  engine: EngineChoice
  workerId?: string
  value: string
  onChange: (value: string) => void
  disabled?: boolean
}

export function ModelField({ id, engine, workerId, value, onChange, disabled }: ModelFieldProps) {
  const { t } = useTranslation()
  const [activated, setActivated] = useState(false)
  const { models, status } = useEngineModels(engine.id, workerId, activated)
  const notices = useMemo<ModelNotices>(
    () => ({
      loading: t("workers.form.modelLoading"),
      error: t("workers.form.modelListError"),
      noMatches: t("workers.form.modelNoMatches"),
    }),
    [t],
  )

  return (
    <ModelCombobox
      id={id}
      label={t("workers.form.model")}
      description={t("workers.form.modelHelper", { engine: engine.name })}
      placeholder={t("workers.form.modelPlaceholder")}
      value={value}
      onChange={onChange}
      models={models}
      status={status}
      notices={notices}
      onFocus={() => setActivated(true)}
      disabled={disabled}
    />
  )
}
