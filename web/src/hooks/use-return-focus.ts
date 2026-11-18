import { useLayoutEffect, useRef } from "react"

export function useReturnFocus(open: boolean) {
  const origin = useRef<HTMLElement | null>(null)
  const kept = useRef(false)

  useLayoutEffect(() => {
    if (!open) return
    kept.current = false
    origin.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
  }, [open])

  return {
    keepFocus: () => {
      kept.current = true
    },
    onCloseAutoFocus: (event: Event) => {
      event.preventDefault()
      const target = origin.current
      origin.current = null
      if (!kept.current && target?.isConnected) target.focus({ preventScroll: true })
    },
  }
}
