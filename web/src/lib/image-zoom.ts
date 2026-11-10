export type Size = { width: number; height: number }

export const MIN_ZOOM = 0.1
export const MAX_ZOOM = 8
export const LONG_IMAGE_RATIO = 2

const STEPS = [0.1, 0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4, 6, 8]
const EPSILON = 1e-3

export function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

export function sameZoom(a: number, b: number) {
  return Math.abs(a - b) < EPSILON
}

export function isLongImage(image: Size) {
  return image.height > LONG_IMAGE_RATIO * image.width
}

export function containScale(image: Size, box: Size) {
  return Math.min(1, box.width / image.width, box.height / image.height)
}

export function fitScale(image: Size, box: Size) {
  return isLongImage(image) ? Math.min(1, box.width / image.width) : containScale(image, box)
}

export function zoomRange(image: Size, box: Size): [number, number] {
  return [Math.min(MIN_ZOOM, containScale(image, box)), MAX_ZOOM]
}

export function stepZoom(scale: number, direction: 1 | -1, [min, max]: [number, number]) {
  const next =
    direction > 0
      ? STEPS.find((step) => step > scale + EPSILON)
      : [...STEPS].reverse().find((step) => step < scale - EPSILON)
  return clamp(next ?? (direction > 0 ? max : min), min, max)
}

export function toggleZoom(scale: number, fit: number) {
  if (!sameZoom(scale, fit)) return fit
  return fit < 1 ? 1 : 2
}

export function wheelZoomFactor(deltaY: number, deltaMode: number) {
  if (deltaMode === 0 && Math.abs(deltaY) < 50) return Math.exp(-deltaY / 100)
  return deltaY < 0 ? 1.25 : 0.8
}

export function zoomPercent(scale: number) {
  return `${Math.round(scale * 100)}%`
}
