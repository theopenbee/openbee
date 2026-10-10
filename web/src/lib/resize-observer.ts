const callbacks = new WeakMap<Element, () => void>()
let observer: ResizeObserver | null = null

function getObserver() {
  if (!observer) {
    observer = new ResizeObserver((entries) => {
      for (const entry of entries) callbacks.get(entry.target)?.()
    })
  }
  return observer
}

export function observeResize(el: Element, callback: () => void) {
  const shared = getObserver()
  callbacks.set(el, callback)
  shared.observe(el)
  return () => {
    callbacks.delete(el)
    shared.unobserve(el)
  }
}
