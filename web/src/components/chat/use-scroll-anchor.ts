import { useCallback, useEffect, useLayoutEffect, useRef, type RefObject } from "react"
import { observeResize } from "@/lib/resize-observer"

const NEAR_BOTTOM_PX = 48

type ScrollAnchor = { el: Element; top: number }

export interface ScrollAnchorRefs {
  containerRef: RefObject<HTMLElement | null>
  contentRef: RefObject<HTMLElement | null>
  logRef: RefObject<HTMLElement | null>
}

function isNearBottom(container: HTMLElement) {
  return container.scrollHeight - container.scrollTop - container.clientHeight < NEAR_BOTTOM_PX
}

function findScrollAnchor(container: HTMLElement, log: HTMLElement | null): ScrollAnchor | null {
  if (!log) return null
  const containerTop = container.getBoundingClientRect().top
  const items = log.children
  let lo = 0
  let hi = items.length - 1
  let anchor: Element | null = null
  while (lo <= hi) {
    const mid = (lo + hi) >> 1
    if (items[mid].getBoundingClientRect().bottom > containerTop) {
      anchor = items[mid]
      hi = mid - 1
    } else {
      lo = mid + 1
    }
  }
  return anchor ? { el: anchor, top: anchor.getBoundingClientRect().top - containerTop } : null
}

export function useScrollAnchor({ containerRef, contentRef, logRef }: ScrollAnchorRefs, dep: unknown) {
  const stickToBottom = useRef(true)
  const scrollAnchorRef = useRef<ScrollAnchor | null>(null)
  const lastScrollTopRef = useRef(0)

  const captureAnchor = useCallback(() => {
    const container = containerRef.current
    scrollAnchorRef.current =
      container && !stickToBottom.current ? findScrollAnchor(container, logRef.current) : null
  }, [containerRef, logRef])

  const syncScroll = useCallback(() => {
    const container = containerRef.current
    if (!container) return
    if (stickToBottom.current) {
      container.scrollTop = container.scrollHeight
      lastScrollTopRef.current = container.scrollTop
      return
    }
    const anchor = scrollAnchorRef.current
    if (anchor?.el.isConnected) {
      const delta = anchor.el.getBoundingClientRect().top - container.getBoundingClientRect().top - anchor.top
      if (Math.abs(delta) >= 1) container.scrollTop += delta
    }
    lastScrollTopRef.current = container.scrollTop
    captureAnchor()
  }, [containerRef, captureAnchor])

  useEffect(() => {
    const container = containerRef.current
    const content = contentRef.current
    if (!container || !content) return
    const stopContainer = observeResize(container, syncScroll)
    const stopContent = observeResize(content, syncScroll)
    return () => {
      stopContainer()
      stopContent()
    }
  }, [containerRef, contentRef, syncScroll])

  useLayoutEffect(() => {
    syncScroll()
  }, [dep, syncScroll])

  const handleScroll = useCallback(() => {
    const container = containerRef.current
    if (!container) return
    const top = container.scrollTop
    if (Math.abs(top - lastScrollTopRef.current) < 1) {
      syncScroll()
      return
    }
    const movedUp = top < lastScrollTopRef.current
    lastScrollTopRef.current = top
    if (isNearBottom(container)) stickToBottom.current = true
    else if (movedUp) stickToBottom.current = false
    captureAnchor()
  }, [containerRef, captureAnchor, syncScroll])

  return { handleScroll, captureAnchor, stickToBottom }
}
