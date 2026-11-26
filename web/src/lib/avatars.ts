import { tokenParam } from "./auth"
import { config } from "./config"
import type { AvatarPack } from "./types"

export function avatarUrl(ref: string): string {
  const path = ref.split("/").map(encodeURIComponent).join("/")
  return `${config.apiUrl}/avatars/${path}${tokenParam()}`
}

export function avatarLabel(ref: string): string {
  return ref.slice(ref.indexOf("/") + 1)
}

export function pickRandomAvatar(packs: readonly AvatarPack[], used: ReadonlySet<string>, random: () => number = Math.random): string {
  const all = packs.flatMap((pack) => pack.avatars)
  const free = all.filter((ref) => !used.has(ref))
  const pool = free.length > 0 ? free : all
  if (pool.length === 0) return ""
  return pool[Math.min(pool.length - 1, Math.floor(random() * pool.length))]
}
