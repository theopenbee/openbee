import { describe, expect, it } from "vitest"
import { formatEngineLabel, formatListTime } from "../format"

const at = (year: number, month: number, day: number, hour = 12, minute = 0) =>
  new Date(year, month - 1, day, hour, minute).getTime()

describe("formatListTime", () => {
  const now = at(2026, 10, 27, 15, 30)

  it("formats today, yesterday, this year and older", () => {
    expect(formatListTime(at(2026, 10, 27, 9, 5), now, "en-GB", "Yesterday")).toBe("09:05")
    expect(formatListTime(at(2026, 10, 26, 23, 59), now, "en-GB", "Yesterday")).toBe("Yesterday")
    expect(formatListTime(at(2026, 3, 4, 8, 0), now, "en-GB", "Yesterday")).toBe("04/03")
    expect(formatListTime(at(2025, 12, 31, 8, 0), now, "en-GB", "Yesterday")).toBe("31/12/2025")
  })

  it("treats the local midnight boundary as a day change", () => {
    const justAfterMidnight = at(2026, 10, 27, 0, 1)
    expect(formatListTime(at(2026, 10, 27, 0, 0), justAfterMidnight, "en-GB", "Yesterday")).toBe("00:00")
    expect(formatListTime(at(2026, 10, 26, 23, 59), justAfterMidnight, "en-GB", "Yesterday")).toBe("Yesterday")
    expect(formatListTime(at(2026, 10, 25, 23, 59), justAfterMidnight, "en-GB", "Yesterday")).toBe("25/10")
  })

  it("finds yesterday across month and year boundaries", () => {
    expect(formatListTime(at(2026, 2, 28, 20, 0), at(2026, 3, 1, 8, 0), "en-GB", "Yesterday")).toBe("Yesterday")
    expect(formatListTime(at(2025, 12, 31, 20, 0), at(2026, 1, 1, 8, 0), "en-GB", "Yesterday")).toBe("Yesterday")
    expect(formatListTime(at(2024, 2, 29, 20, 0), at(2024, 3, 1, 8, 0), "en-GB", "Yesterday")).toBe("Yesterday")
  })

  it("uses the given yesterday label and locale", () => {
    expect(formatListTime(at(2026, 10, 26, 10, 0), now, "zh-CN", "昨天")).toBe("昨天")
    expect(formatListTime(at(2026, 3, 4, 8, 0), now, "zh-CN", "昨天")).toBe("3/4")
    expect(formatListTime(at(2025, 3, 4, 8, 0), now, "zh-CN", "昨天")).toBe("2025/3/4")
  })
})

describe("formatEngineLabel", () => {
  const engines = [{ id: "pi", name: "Pi" }]

  it("uses the localized engine name", () => {
    expect(formatEngineLabel("pi", engines)).toBe("Pi")
  })

  it("falls back to the id when the engine is not installed", () => {
    expect(formatEngineLabel("codex", engines)).toBe("codex")
    expect(formatEngineLabel("codex", [])).toBe("codex")
  })

  it("shows a dash for an unset engine", () => {
    expect(formatEngineLabel("", engines)).toBe("—")
    expect(formatEngineLabel(undefined, engines)).toBe("—")
    expect(formatEngineLabel(null, engines)).toBe("—")
  })
})
