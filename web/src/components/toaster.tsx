import { Toaster as Sonner } from "sonner"
import { useThemePreference } from "@/hooks/use-theme-toggle"

export function Toaster() {
  const { theme } = useThemePreference()
  return (
    <Sonner
      theme={theme}
      position="bottom-right"
      gap={8}
      toastOptions={{
        unstyled: true,
        classNames: {
          toast:
            "flex w-full items-start gap-3 rounded-panel border border-border bg-surface-raised px-4 py-3 text-sm text-foreground shadow-floating sm:w-[22rem]",
          title: "font-medium leading-snug",
          description: "mt-0.5 text-secondary",
          icon: "mt-0.5 shrink-0 [&_svg]:size-4",
          success: "[&_[data-icon]]:text-success",
          error: "[&_[data-icon]]:text-danger",
          warning: "[&_[data-icon]]:text-warning",
          info: "[&_[data-icon]]:text-accent",
          actionButton:
            "ml-auto shrink-0 rounded-control bg-foreground px-3 py-1.5 text-xs font-medium text-background",
          cancelButton:
            "shrink-0 rounded-control border border-border px-3 py-1.5 text-xs font-medium text-secondary",
          closeButton: "text-secondary",
        },
      }}
    />
  )
}
