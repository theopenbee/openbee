import {
  useEffect,
  useEffectEvent,
  useLayoutEffect,
  useRef,
  useState,
  type MouseEvent,
  type PointerEvent,
} from "react"
import { Scan, ZoomIn, ZoomOut } from "lucide-react"
import { useTranslation } from "react-i18next"
import { Button } from "@/components/arc/button/button"
import { DownloadLink, MediaViewer, VIEWER_KEEP, VIEWER_LINK } from "@/components/media-viewer"
import {
  clamp,
  fitScale,
  sameZoom,
  stepZoom,
  toggleZoom,
  wheelZoomFactor,
  zoomPercent,
  zoomRange,
  type Size,
} from "@/lib/image-zoom"
import { observeResize } from "@/lib/resize-observer"
import { cn } from "@/lib/utils"

const PAD_NARROW = 16
const PAD_WIDE = 48
const PAD_BOTTOM = 32
const WIDE_VIEW = 640
const DRAG_SLOP = 4
const BORDER = 1

type Point = { x: number; y: number }
type Anchor = Point & { u: number; v: number }
type Drag = { id: number; x: number; y: number; left: number; top: number; moved: boolean; image: boolean }
type Pinch = { distance: number; scale: number; u: number; v: number }
type GestureEvent = UIEvent & { scale: number; clientX: number; clientY: number }

function distance(a: Touch, b: Touch) {
  return Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY)
}

function useImageZoom(close: () => void) {
  const [scroller, setScroller] = useState<HTMLDivElement | null>(null)
  const image = useRef<HTMLImageElement>(null)
  const [view, setView] = useState<Size | null>(null)
  const [natural, setNatural] = useState<Size | null>(null)
  const [settled, setSettled] = useState(false)
  const [failed, setFailed] = useState(false)
  const [zoom, setZoom] = useState<number | null>(null)
  const [dragging, setDragging] = useState(false)
  const rendered = useRef(1)
  const target = useRef(1)
  const anchor = useRef<Anchor | null>(null)
  const drag = useRef<Drag | null>(null)
  const pinch = useRef<Pinch | null>(null)
  const gesture = useRef<number | null>(null)

  const pad = view && view.width >= WIDE_VIEW ? PAD_WIDE : PAD_NARROW
  const box = view && {
    width: Math.max(1, view.width - 2 * pad - 2 * BORDER),
    height: Math.max(1, view.height - PAD_BOTTOM - 2 * BORDER),
  }
  const size = natural ?? (settled && !failed ? box : null)
  const fit = size && box ? fitScale(size, box) : 1
  const range = size && box ? zoomRange(size, box) : null
  const scale = zoom ?? fit
  const outer = size && { width: size.width * scale + 2 * BORDER, height: size.height * scale + 2 * BORDER }
  const overflows =
    !!outer && !!view && (outer.width + 2 * pad > view.width + 1 || outer.height + PAD_BOTTOM > view.height + 1)

  useLayoutEffect(() => {
    if (!scroller) return
    const measure = () => setView({ width: scroller.clientWidth, height: scroller.clientHeight })
    measure()
    return observeResize(scroller, measure)
  }, [scroller])

  useLayoutEffect(() => {
    rendered.current = scale
    target.current = scale
    const a = anchor.current
    const img = image.current
    anchor.current = null
    if (!a || !scroller || !img) return
    scroller.scrollLeft = a.u * scale + img.offsetLeft + BORDER - a.x
    scroller.scrollTop = a.v * scale + img.offsetTop + BORDER - a.y
  }, [scale, scroller])

  const localPoint = (clientX: number, clientY: number): Point => {
    const rect = scroller!.getBoundingClientRect()
    return { x: clientX - rect.left, y: clientY - rect.top }
  }

  const imagePoint = (at: Point) => {
    const img = image.current!
    return {
      u: (scroller!.scrollLeft + at.x - img.offsetLeft - BORDER) / rendered.current,
      v: (scroller!.scrollTop + at.y - img.offsetTop - BORDER) / rendered.current,
    }
  }

  const zoomTo = (next: number, at?: Point, held?: { u: number; v: number }) => {
    if (!range || !scroller || !image.current) return
    const value = clamp(next, range[0], range[1])
    if (sameZoom(value, target.current)) return
    const point = at ?? { x: scroller.clientWidth / 2, y: scroller.clientHeight / 2 }
    anchor.current = { ...point, ...(held ?? imagePoint(point)) }
    target.current = value
    setZoom(sameZoom(value, fit) ? null : value)
  }

  const step = (direction: 1 | -1) => {
    if (range) zoomTo(stepZoom(target.current, direction, range))
  }
  const reset = () => zoomTo(fit)

  const onWheel = useEffectEvent((e: WheelEvent) => {
    if (!e.ctrlKey && !e.metaKey) return
    e.preventDefault()
    zoomTo(target.current * wheelZoomFactor(e.deltaY, e.deltaMode), localPoint(e.clientX, e.clientY))
  })

  const onTouchStart = useEffectEvent((e: TouchEvent) => {
    if (e.touches.length !== 2 || !scroller || !image.current) {
      pinch.current = null
      return
    }
    const [a, b] = [e.touches[0], e.touches[1]]
    const mid = localPoint((a.clientX + b.clientX) / 2, (a.clientY + b.clientY) / 2)
    pinch.current = { distance: distance(a, b), scale: target.current, ...imagePoint(mid) }
  })

  const onTouchMove = useEffectEvent((e: TouchEvent) => {
    const p = pinch.current
    if (!p || e.touches.length !== 2) return
    e.preventDefault()
    const [a, b] = [e.touches[0], e.touches[1]]
    const mid = localPoint((a.clientX + b.clientX) / 2, (a.clientY + b.clientY) / 2)
    zoomTo((p.scale * distance(a, b)) / p.distance, mid, p)
  })

  const onTouchEnd = useEffectEvent((e: TouchEvent) => {
    if (e.touches.length < 2) pinch.current = null
  })

  const onGestureStart = useEffectEvent((e: Event) => {
    e.preventDefault()
    gesture.current = pinch.current ? null : target.current
  })

  const onGestureChange = useEffectEvent((e: Event) => {
    e.preventDefault()
    const base = gesture.current
    if (base === null || pinch.current) return
    const g = e as GestureEvent
    zoomTo(base * g.scale, localPoint(g.clientX, g.clientY))
  })

  const onKey = useEffectEvent((e: KeyboardEvent) => {
    if (e.ctrlKey || e.metaKey || e.altKey) return
    if (e.key === "+" || e.key === "=") step(1)
    else if (e.key === "-" || e.key === "_") step(-1)
    else if (e.key === "0") reset()
    else return
    e.preventDefault()
  })

  useEffect(() => {
    if (!scroller) return
    const wheel = (e: WheelEvent) => onWheel(e)
    const touchStart = (e: TouchEvent) => onTouchStart(e)
    const touchMove = (e: TouchEvent) => onTouchMove(e)
    const touchEnd = (e: TouchEvent) => onTouchEnd(e)
    const gestureStart = (e: Event) => onGestureStart(e)
    const gestureChange = (e: Event) => onGestureChange(e)
    const key = (e: KeyboardEvent) => onKey(e)
    const active = { passive: false }
    scroller.addEventListener("wheel", wheel, active)
    scroller.addEventListener("touchstart", touchStart, active)
    scroller.addEventListener("touchmove", touchMove, active)
    scroller.addEventListener("touchend", touchEnd)
    scroller.addEventListener("touchcancel", touchEnd)
    scroller.addEventListener("gesturestart", gestureStart, active)
    scroller.addEventListener("gesturechange", gestureChange, active)
    window.addEventListener("keydown", key)
    return () => {
      scroller.removeEventListener("wheel", wheel)
      scroller.removeEventListener("touchstart", touchStart)
      scroller.removeEventListener("touchmove", touchMove)
      scroller.removeEventListener("touchend", touchEnd)
      scroller.removeEventListener("touchcancel", touchEnd)
      scroller.removeEventListener("gesturestart", gestureStart)
      scroller.removeEventListener("gesturechange", gestureChange)
      window.removeEventListener("keydown", key)
    }
  }, [scroller])

  const endDrag = () => {
    drag.current = null
    setDragging(false)
  }

  const onPointerDown = (e: PointerEvent<HTMLDivElement>) => {
    const el = e.currentTarget
    const at = localPoint(e.clientX, e.clientY)
    if (at.x >= el.clientWidth || at.y >= el.clientHeight) {
      e.stopPropagation()
      return
    }
    if (e.pointerType !== "mouse" || e.button !== 0 || !overflows) return
    e.stopPropagation()
    drag.current = {
      id: e.pointerId,
      x: e.clientX,
      y: e.clientY,
      left: el.scrollLeft,
      top: el.scrollTop,
      moved: false,
      image: e.target === image.current,
    }
  }

  const onPointerMove = (e: PointerEvent<HTMLDivElement>) => {
    const d = drag.current
    if (!d || d.id !== e.pointerId) return
    if ((e.buttons & 1) === 0) {
      endDrag()
      return
    }
    const dx = e.clientX - d.x
    const dy = e.clientY - d.y
    if (!d.moved) {
      if (Math.hypot(dx, dy) < DRAG_SLOP) return
      d.moved = true
      e.currentTarget.setPointerCapture(e.pointerId)
      setDragging(true)
    }
    e.currentTarget.scrollLeft = d.left - dx
    e.currentTarget.scrollTop = d.top - dy
  }

  const onPointerEnd = (e: PointerEvent<HTMLDivElement>) => {
    const d = drag.current
    if (!d || d.id !== e.pointerId) return
    endDrag()
    if (e.type === "pointerup" && !d.moved && !d.image) close()
  }

  const onDoubleClick = (e: MouseEvent) => {
    zoomTo(toggleZoom(scale, fit), localPoint(e.clientX, e.clientY))
  }

  const onLoad = (img: HTMLImageElement) => {
    setSettled(true)
    setFailed(false)
    if (img.naturalWidth > 0 && img.naturalHeight > 0) {
      setNatural({ width: img.naturalWidth, height: img.naturalHeight })
    }
  }

  return {
    scroller: setScroller,
    image,
    pad,
    outer,
    settled,
    scale,
    fitted: zoom === null,
    range,
    overflows,
    dragging,
    step,
    reset,
    restart: () => {
      endDrag()
      setZoom(null)
    },
    stage: { onPointerDown, onPointerMove, onPointerUp: onPointerEnd, onPointerCancel: onPointerEnd },
    onDoubleClick,
    onLoad,
    onError: () => {
      setSettled(true)
      setFailed(true)
    },
  }
}

export function ImageLightbox({
  src,
  alt,
  className,
}: {
  src: string
  alt: string
  className?: string
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const zoom = useImageZoom(() => setOpen(false))

  const onOpenChange = (next: boolean) => {
    if (next) zoom.restart()
    setOpen(next)
  }

  return (
    <MediaViewer
      open={open}
      onOpenChange={onOpenChange}
      title={alt}
      closeLabel={t("chat.closeImage")}
      trigger={
        <button
          type="button"
          aria-label={t("chat.viewImage", { name: alt })}
          className={cn(
            "block max-w-full cursor-zoom-in overflow-hidden rounded-inset border border-border bg-surface transition-opacity hover:opacity-90",
            className
          )}
        >
          <img src={src} alt={alt} className="block max-h-60 w-auto max-w-full object-contain" />
        </button>
      }
      action={
        <>
          <div className="hidden items-center gap-1 sm:flex">
            <Button
              variant="secondary"
              aria-label={t("chat.zoomOut")}
              disabled={!zoom.range || zoom.scale <= zoom.range[0]}
              onClick={() => zoom.step(-1)}
            >
              <ZoomOut size={16} strokeWidth={1.75} aria-hidden="true" />
            </Button>
            <output aria-live="polite" className="min-w-14 text-center text-sm tabular-nums text-secondary">
              {zoomPercent(zoom.scale)}
            </output>
            <Button
              variant="secondary"
              aria-label={t("chat.zoomIn")}
              disabled={!zoom.range || zoom.scale >= zoom.range[1]}
              onClick={() => zoom.step(1)}
            >
              <ZoomIn size={16} strokeWidth={1.75} aria-hidden="true" />
            </Button>
          </div>
          <Button variant="secondary" aria-label={t("chat.zoomFit")} disabled={zoom.fitted} onClick={zoom.reset}>
            <Scan size={16} strokeWidth={1.75} aria-hidden="true" />
          </Button>
          <DownloadLink url={src} filename={alt} className={VIEWER_LINK} compact />
        </>
      }
    >
      <div
        ref={zoom.scroller}
        {...zoom.stage}
        className={cn(
          "relative min-h-0 flex-1 touch-pan-x touch-pan-y overflow-auto overscroll-contain [scrollbar-gutter:stable]",
          zoom.overflows && "cursor-grab",
          zoom.dragging && "cursor-grabbing select-none"
        )}
      >
        <div
          className="flex min-h-full w-max min-w-full"
          style={{ paddingLeft: zoom.pad, paddingRight: zoom.pad, paddingBottom: PAD_BOTTOM }}
        >
          <img
            {...VIEWER_KEEP}
            ref={zoom.image}
            src={src}
            alt={alt}
            draggable={false}
            onLoad={(e) => zoom.onLoad(e.currentTarget)}
            onError={zoom.onError}
            onDoubleClick={zoom.onDoubleClick}
            style={zoom.outer ?? undefined}
            className={cn(
              "m-auto block max-w-none shrink-0 rounded-panel border border-border bg-surface object-contain shadow-floating",
              !zoom.settled && "invisible"
            )}
          />
        </div>
      </div>
    </MediaViewer>
  )
}
