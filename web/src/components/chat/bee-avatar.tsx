import { LogoMark } from "@/components/brand/logo"
import type { AvatarSize } from "@/components/worker-avatar"
import { cn } from "@/lib/utils"

interface BeeAvatarProps {
  size?: AvatarSize
  className?: string
}

const dimensions: Record<AvatarSize, string> = {
  sm: "size-7 p-1",
  default: "size-9 p-1.5",
  lg: "size-12 p-2",
  xl: "size-22 p-3.5",
}

export function BeeAvatar({ size = "default", className }: BeeAvatarProps) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-full border border-border bg-avatar-bee",
        dimensions[size],
        className,
      )}
    >
      <LogoMark />
    </span>
  )
}
