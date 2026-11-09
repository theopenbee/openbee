import { afterEach, describe, expect, it } from "vitest"
import { clearDrafts, readDraft, saveDraft } from "../drafts"

afterEach(clearDrafts)

describe("drafts", () => {
  it("keeps each conversation's draft apart", () => {
    saveDraft("bee", { text: "hi bee", media: [] })
    saveDraft("worker:w1", { text: "", media: ["/tmp/a.png"] })
    expect(readDraft("bee")).toEqual({ text: "hi bee", media: [] })
    expect(readDraft("worker:w1")).toEqual({ text: "", media: ["/tmp/a.png"] })
    expect(readDraft("worker:w2")).toEqual({ text: "", media: [] })
  })

  it("forgets a draft once it is empty", () => {
    saveDraft("bee", { text: "hi", media: [] })
    saveDraft("bee", { text: "", media: [] })
    expect(readDraft("bee")).toEqual({ text: "", media: [] })
  })

  it("clears every draft", () => {
    saveDraft("bee", { text: "hi", media: [] })
    clearDrafts()
    expect(readDraft("bee")).toEqual({ text: "", media: [] })
  })
})
