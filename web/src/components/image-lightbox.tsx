import { useState } from "react"
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog"
import { ArrowUpRight, X } from "lucide-react"

import { cn } from "@/lib/utils"

// A click-to-zoom image: renders a bordered thumbnail that opens the image
// full-screen in a dialog overlay. Uses base-ui's Dialog primitives directly
// (rather than the small centered DialogContent card) so the viewer can fill
// the viewport. The portal renders at the document root, so the message
// bubble's `overflow-hidden` never clips it.
export function ImageLightbox({
  src,
  alt,
  className,
}: {
  src: string
  alt: string
  className?: string
}) {
  const [open, setOpen] = useState(false)

  return (
    <DialogPrimitive.Root open={open} onOpenChange={setOpen}>
      <DialogPrimitive.Trigger
        render={<button type="button" />}
        className={cn(
          "block cursor-zoom-in overflow-hidden rounded-sm border",
          className
        )}
      >
        <img src={src} alt={alt} className="max-h-80 w-full object-contain" />
      </DialogPrimitive.Trigger>

      <DialogPrimitive.Portal>
        <DialogPrimitive.Backdrop
          className="fixed inset-0 z-50 bg-black/80 duration-100 supports-backdrop-filter:backdrop-blur-xs data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0"
        />
        <DialogPrimitive.Popup
          // Clicking the empty area around the image dismisses; clicks on the
          // image itself are stopped so it stays open.
          onClick={() => setOpen(false)}
          className="fixed inset-0 z-50 flex items-center justify-center p-4 outline-none duration-100 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"
        >
          <img
            src={src}
            alt={alt}
            onClick={(e) => e.stopPropagation()}
            className="max-h-[90vh] max-w-[90vw] rounded-sm object-contain"
          />

          <a
            href={src}
            target="_blank"
            rel="noreferrer"
            onClick={(e) => e.stopPropagation()}
            className="absolute top-4 right-16 inline-flex size-9 items-center justify-center rounded-sm text-white/80 transition-colors hover:bg-white/10 hover:text-white"
          >
            <ArrowUpRight className="size-5" />
            <span className="sr-only">Open original in new tab</span>
          </a>

          <DialogPrimitive.Close
            className="absolute top-4 right-4 inline-flex size-9 items-center justify-center rounded-sm text-white/80 transition-colors hover:bg-white/10 hover:text-white"
          >
            <X className="size-5" />
            <span className="sr-only">Close</span>
          </DialogPrimitive.Close>
        </DialogPrimitive.Popup>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}
