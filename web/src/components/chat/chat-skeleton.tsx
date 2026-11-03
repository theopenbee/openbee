export function ChatSkeleton() {
  return (
    <div className="flex flex-col gap-8" aria-busy="true">
      <div className="flex justify-end">
        <div className="skeleton h-11 w-2/5" />
      </div>
      <div className="flex flex-col gap-2">
        <div className="skeleton h-3 w-12" />
        <div className="skeleton h-4 w-full" />
        <div className="skeleton h-4 w-4/5" />
        <div className="skeleton h-4 w-3/5" />
      </div>
      <div className="flex justify-end">
        <div className="skeleton h-11 w-1/3" />
      </div>
      <div className="flex flex-col gap-2">
        <div className="skeleton h-3 w-12" />
        <div className="skeleton h-4 w-11/12" />
        <div className="skeleton h-4 w-2/3" />
      </div>
    </div>
  )
}
