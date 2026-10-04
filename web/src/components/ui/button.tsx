import { Button as ButtonPrimitive } from "@base-ui/react/button"
import type { ButtonProps } from "@base-ui/react/button"
import { cva, type VariantProps } from "class-variance-authority"

import { cn } from "@/lib/utils"

// Kumo button grammar: a 1px ring instead of a border, a hairline drop shadow,
// one blue emphasis fill for the primary action, and squared corners (≤ sm).
const buttonVariants = cva(
  "group/button inline-flex shrink-0 cursor-pointer items-center justify-center rounded-sm text-sm font-medium whitespace-nowrap transition-[color,background-color,box-shadow] duration-150 outline-none select-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 aria-invalid:ring-destructive [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default:
          "bg-primary bg-linear-to-b from-white/8 to-transparent text-primary-foreground shadow-xs ring-1 ring-primary-edge hover:bg-primary-hover focus-visible:ring-offset-1 focus-visible:ring-offset-background",
        outline:
          "bg-background text-foreground shadow-xs ring-1 ring-border hover:bg-accent hover:text-strong aria-expanded:bg-accent",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-recessed aria-expanded:bg-recessed",
        ghost:
          "text-foreground hover:bg-accent hover:text-strong aria-expanded:bg-accent",
        destructive:
          "bg-destructive bg-linear-to-b from-white/8 to-transparent text-white shadow-xs ring-1 ring-destructive hover:bg-[color-mix(in_oklch,var(--destructive),black_10%)] focus-visible:ring-offset-1 focus-visible:ring-offset-background",
        link: "text-link underline-offset-4 hover:underline",
      },
      size: {
        default:
          "h-9 gap-1.5 px-3 has-data-[icon=inline-end]:pr-2.5 has-data-[icon=inline-start]:pl-2.5",
        xs: "h-5 gap-1 px-1.5 text-xs [&_svg:not([class*='size-'])]:size-3",
        sm: "h-7 gap-1 px-2 text-xs has-data-[icon=inline-end]:pr-1.5 has-data-[icon=inline-start]:pl-1.5 [&_svg:not([class*='size-'])]:size-3.5",
        lg: "h-10 gap-2 px-4",
        icon: "size-9",
        "icon-xs": "size-6 [&_svg:not([class*='size-'])]:size-3.5",
        "icon-sm": "size-7 [&_svg:not([class*='size-'])]:size-4",
        "icon-lg": "size-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  ...props
}: ButtonProps & VariantProps<typeof buttonVariants>) {
  return (
    <ButtonPrimitive
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
