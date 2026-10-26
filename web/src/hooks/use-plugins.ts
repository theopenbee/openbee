import { useCallback, useEffect, useRef, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { getRefreshToken, refreshAccessToken, tokenParam } from "@/lib/auth"
import { isTransitional } from "@/lib/plugins"
import type { PluginOperation, PluginSettingsUpdate, PluginSetupEvent } from "@/lib/types"

const TRANSITION_POLL_MS = 2000
const OPERATION_POLL_MS = 1000
const MAX_SETUP_RECONNECTS = 5

export function usePlugins() {
  return useQuery({
    queryKey: ["plugins"],
    queryFn: api.plugins.list,
    refetchInterval: (query) =>
      query.state.data?.some((p) => isTransitional(p.state)) ? TRANSITION_POLL_MS : false,
  })
}

export function usePlugin(id: string) {
  return useQuery({
    queryKey: ["plugins", id],
    queryFn: () => api.plugins.get(id),
    refetchInterval: (query) =>
      query.state.data && isTransitional(query.state.data.state) ? TRANSITION_POLL_MS : false,
  })
}

export function usePluginCatalog() {
  return useQuery({
    queryKey: ["plugin-catalog"],
    queryFn: () => api.plugins.catalog(),
    retry: false,
  })
}

export function useRefreshPluginCatalog() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => api.plugins.catalog(true),
    onSuccess: (data) => queryClient.setQueryData(["plugin-catalog"], data),
  })
}

function isPending(op: PluginOperation) {
  return op.status === "queued" || op.status === "running"
}

export function usePluginOperations(onFinished?: (op: PluginOperation) => void) {
  const queryClient = useQueryClient()
  const onFinishedRef = useRef(onFinished)
  onFinishedRef.current = onFinished
  const pendingRef = useRef<Set<string> | null>(null)
  const query = useQuery({
    queryKey: ["plugin-operations"],
    queryFn: api.plugins.operations,
    refetchInterval: (q) => (q.state.data?.some(isPending) ? OPERATION_POLL_MS : false),
  })

  useEffect(() => {
    const ops = query.data
    if (!ops) return
    const previous = pendingRef.current
    pendingRef.current = new Set(ops.filter(isPending).map((op) => op.id))
    if (!previous) return
    const finished = ops.filter((op) => previous.has(op.id) && !isPending(op))
    if (finished.length === 0) return
    queryClient.invalidateQueries({ queryKey: ["plugins"] })
    queryClient.invalidateQueries({ queryKey: ["plugin-catalog"] })
    finished.forEach((op) => onFinishedRef.current?.(op))
  }, [query.data, queryClient])

  return query
}

export function useInstallPlugin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (spec: string) => api.plugins.install(spec),
    onSuccess: (op) => {
      queryClient.setQueryData<PluginOperation[]>(["plugin-operations"], (old) => [op, ...(old ?? [])])
      queryClient.invalidateQueries({ queryKey: ["plugin-operations"] })
    },
  })
}

export function useUninstallPlugin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.plugins.uninstall(id),
    onSuccess: (op) => {
      queryClient.setQueryData<PluginOperation[]>(["plugin-operations"], (old) => [op, ...(old ?? [])])
      queryClient.invalidateQueries({ queryKey: ["plugin-operations"] })
    },
  })
}

export type PluginAction = "enable" | "disable" | "restart"

export function usePluginAction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, action }: { id: string; action: PluginAction }) => api.plugins[action](id),
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["plugins"] }),
  })
}

export function useUpdatePluginSettings(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: PluginSettingsUpdate) => api.plugins.updateSettings(id, data),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["plugins"] }),
  })
}

export interface SetupState {
  sessionId: string | null
  lines: string[]
  done: PluginSetupEvent | null
  connected: boolean
}

const IDLE_SETUP: SetupState = { sessionId: null, lines: [], done: null, connected: false }

export function usePluginSetup(pluginId: string, onDone?: (ev: PluginSetupEvent) => void) {
  const queryClient = useQueryClient()
  const [state, setState] = useState<SetupState>(IDLE_SETUP)
  const onDoneRef = useRef(onDone)
  onDoneRef.current = onDone
  const sessionId = state.sessionId

  useEffect(() => {
    if (!sessionId) return
    let es: EventSource | null = null
    let timer: ReturnType<typeof setTimeout> | undefined
    let closed = false
    let attempts = 0

    const connect = () => {
      if (closed) return
      es = new EventSource(`${api.plugins.setupEventsUrl(sessionId)}${tokenParam()}`)
      es.onopen = () => {
        attempts = 0
        setState((s) => (s.sessionId === sessionId ? { ...s, lines: [], connected: true } : s))
      }
      es.onmessage = (event) => {
        let ev: PluginSetupEvent
        try {
          ev = JSON.parse(event.data)
        } catch {
          return
        }
        if (ev.type === "line" && ev.text) {
          const text = ev.text
          setState((s) => (s.sessionId === sessionId ? { ...s, lines: [...s.lines, text] } : s))
          return
        }
        if (ev.type === "done") {
          closed = true
          es?.close()
          setState((s) => (s.sessionId === sessionId ? { ...s, done: ev, connected: false } : s))
          queryClient.invalidateQueries({ queryKey: ["plugins"] })
          onDoneRef.current?.(ev)
        }
      }
      es.onerror = () => {
        es?.close()
        setState((s) => (s.sessionId === sessionId ? { ...s, connected: false } : s))
        if (closed || attempts >= MAX_SETUP_RECONNECTS) return
        attempts += 1
        timer = setTimeout(async () => {
          if (getRefreshToken()) await refreshAccessToken().catch(() => null)
          connect()
        }, Math.min(1000 * 2 ** (attempts - 1), 10_000))
      }
    }

    connect()
    return () => {
      closed = true
      clearTimeout(timer)
      es?.close()
    }
  }, [sessionId, queryClient])

  const stateRef = useRef(state)
  stateRef.current = state

  const start = useCallback(async () => {
    const { id } = await api.plugins.startSetup(pluginId)
    setState({ ...IDLE_SETUP, sessionId: id })
    return id
  }, [pluginId])

  const cancel = useCallback(async () => {
    const { sessionId: sid, done } = stateRef.current
    setState(IDLE_SETUP)
    if (sid && !done) await api.plugins.cancelSetup(sid).catch(() => undefined)
  }, [])

  const input = useCallback(async (text: string) => {
    const sid = stateRef.current.sessionId
    if (sid) await api.plugins.setupInput(sid, text)
  }, [])

  return { ...state, start, cancel, input }
}
