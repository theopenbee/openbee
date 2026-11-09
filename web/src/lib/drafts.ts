export type Draft = { text: string; media: string[] }

const EMPTY: Draft = { text: "", media: [] }
const drafts = new Map<string, Draft>()

export function readDraft(key: string): Draft {
  return drafts.get(key) ?? EMPTY
}

export function saveDraft(key: string, draft: Draft): void {
  if (draft.text === "" && draft.media.length === 0) drafts.delete(key)
  else drafts.set(key, draft)
}

export function clearDrafts(): void {
  drafts.clear()
}
