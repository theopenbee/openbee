const AVATAR_CLASSES = [
  "bg-avatar-1",
  "bg-avatar-2",
  "bg-avatar-3",
  "bg-avatar-4",
  "bg-avatar-5",
  "bg-avatar-6",
  "bg-avatar-7",
  "bg-avatar-8",
] as const

export function avatarColorIndex(id: string): number {
  let hash = 0x811c9dc5
  for (let i = 0; i < id.length; i++) {
    hash ^= id.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193) >>> 0
  }
  return (hash >>> 0) % AVATAR_CLASSES.length + 1
}

export function avatarColorClass(id: string): string {
  return AVATAR_CLASSES[avatarColorIndex(id) - 1]
}
