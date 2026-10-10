const callbacks = new WeakMap<Element, Set<() => void>>()
let observer: ResizeObserver | null = null

function getObserver() {
  if (!observer) {
    observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        callbacks.get(entry.target)?.forEach((callback) => callback())
      }
    })
  }
  return observer
}

export function observeResize(el: Element, callback: () => void) {
  const shared = getObserver()
  let set = callbacks.get(el)
  if (!set) {
    set = new Set()
    callbacks.set(el, set)
    shared.observe(el)
  }
  set.add(callback)
  return () => {
    const current = callbacks.get(el)
    if (!current?.delete(callback) || current.size > 0) return
    callbacks.delete(el)
    shared.unobserve(el)
  }
}
