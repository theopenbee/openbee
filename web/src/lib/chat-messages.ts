import type { ChatMessage } from "@/lib/types"

export interface ChatState {
  messages: ChatMessage[]
  hasMore: boolean
}

export type ChatAction =
  | { type: "latest"; messages: ChatMessage[]; hasMore: boolean }
  | { type: "older"; messages: ChatMessage[]; hasMore: boolean }
  | { type: "add"; message: ChatMessage }
  | { type: "sent"; id: string; ts: number }
  | { type: "remove"; id: string }

export const EMPTY_CHAT: ChatState = { messages: [], hasMore: false }

export function createMessageId() {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("")
}

function mergeLatest(state: ChatState, latest: ChatMessage[], hasMore: boolean): ChatState {
  const latestIds = new Set(latest.map((message) => message.id))
  const firstTs = latest.length > 0 ? latest[0].ts : -Infinity
  const contiguous = state.messages.some((message) => !message.pending && latestIds.has(message.id))
  const older: ChatMessage[] = []
  const pending: ChatMessage[] = []
  for (const message of state.messages) {
    if (latestIds.has(message.id)) continue
    if (message.pending) {
      if (contiguous || message.ts >= firstTs) pending.push(message)
    } else if (contiguous && message.ts <= firstTs) {
      older.push(message)
    }
  }
  return {
    messages: [...older, ...latest, ...pending].sort((a, b) => a.ts - b.ts),
    hasMore: older.length > 0 ? state.hasMore : hasMore,
  }
}

export function chatReducer(state: ChatState, action: ChatAction): ChatState {
  switch (action.type) {
    case "latest":
      return mergeLatest(state, action.messages, action.hasMore)
    case "older": {
      const ids = new Set(state.messages.map((message) => message.id))
      const older = action.messages.filter((message) => !ids.has(message.id))
      return {
        messages: older.length > 0 ? [...older, ...state.messages] : state.messages,
        hasMore: action.hasMore,
      }
    }
    case "add":
      if (state.messages.some((message) => message.id === action.message.id)) return state
      return { ...state, messages: [...state.messages, action.message] }
    case "sent":
      return {
        ...state,
        messages: state.messages.map((message) =>
          message.id === action.id ? { ...message, ts: action.ts } : message
        ),
      }
    case "remove":
      return { ...state, messages: state.messages.filter((message) => message.id !== action.id) }
  }
}
