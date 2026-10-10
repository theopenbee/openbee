function legacyCopy(value: string) {
  const active = document.activeElement instanceof HTMLElement ? document.activeElement : null
  const textarea = document.createElement("textarea")
  textarea.value = value
  textarea.setAttribute("readonly", "")
  Object.assign(textarea.style, {
    position: "fixed",
    top: "0",
    left: "0",
    opacity: "0",
    pointerEvents: "none",
    fontSize: "16px",
  })
  document.body.appendChild(textarea)
  textarea.select()
  textarea.setSelectionRange(0, value.length)
  try {
    return document.execCommand("copy")
  } catch {
    return false
  } finally {
    textarea.remove()
    active?.focus({ preventScroll: true })
  }
}

export async function copyText(value: string) {
  if (!navigator.clipboard?.writeText) return legacyCopy(value)
  try {
    await navigator.clipboard.writeText(value)
    return true
  } catch {
    return legacyCopy(value)
  }
}
