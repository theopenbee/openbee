import * as React from "react"

import { cn } from "@/lib/utils"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "flex field-sizing-content min-h-16 w-full rounded-sm bg-background px-3 py-2 text-base text-foreground shadow-xs ring-1 ring-input transition-shadow outline-none placeholder:text-muted-foreground focus-visible:ring-[1.5px] focus-visible:ring-focus/50 disabled:cursor-not-allowed disabled:bg-muted disabled:text-muted-foreground aria-invalid:ring-[1.5px] aria-invalid:ring-destructive/60 md:text-sm",
        className
      )}
      {...props}
    />
  )
}

export { Textarea }
