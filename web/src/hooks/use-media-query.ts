import { useCallback, useSyncExternalStore } from "react"

const mediaQueryLists = new Map<string, MediaQueryList>()

function getMediaQueryList(query: string) {
  let mql = mediaQueryLists.get(query)
  if (!mql) {
    mql = window.matchMedia(query)
    mediaQueryLists.set(query, mql)
  }
  return mql
}

export function useMediaQuery(query: string) {
  const mql = getMediaQueryList(query)
  const subscribe = useCallback(
    (onChange: () => void) => {
      mql.addEventListener("change", onChange)
      return () => mql.removeEventListener("change", onChange)
    },
    [mql]
  )
  return useSyncExternalStore(subscribe, () => mql.matches)
}
