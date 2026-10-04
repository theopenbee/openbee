import * as React from "react"

import { cn } from "@/lib/utils"

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "h-9 w-full min-w-0 rounded-sm bg-background px-3 py-1 text-base text-foreground shadow-xs ring-1 ring-input transition-shadow outline-none file:inline-flex file:h-6 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground focus-visible:ring-[1.5px] focus-visible:ring-focus/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:bg-muted disabled:text-muted-foreground aria-invalid:ring-[1.5px] aria-invalid:ring-destructive/60 md:text-sm",
        className
      )}
      {...props}
    />
  )
}

export { Input }
