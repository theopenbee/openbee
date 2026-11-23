import { describe, expect, it } from "vitest"
import {
  CLOSED_SUGGESTIONS,
  buildCreateEngineModels,
  changedEngineModels,
  cleanEngineModels,
  engineModel,
  filterModels,
  highlightedModel,
  modelListNotice,
  normalizeModelList,
  parseEngineModels,
  suggestionReducer,
  visibleModels,
  type SuggestionAction,
  type SuggestionState,
} from "../engine-models"

const MODELS = [
  { id: "deepseek/deepseek-flash", name: "DeepSeek Flash" },
  { id: "deepseek/deepseek-pro" },
  { id: "openai/gpt-5", name: "GPT-5" },
]

const run = (actions: SuggestionAction[], from: SuggestionState = CLOSED_SUGGESTIONS) =>
  actions.reduce(suggestionReducer, from)

describe("normalizeModelList", () => {
  it("returns an empty list for anything that is not an array", () => {
    expect(normalizeModelList(undefined)).toEqual([])
    expect(normalizeModelList(null)).toEqual([])
    expect(normalizeModelList({ id: "x" })).toEqual([])
  })

  it("keeps ids and optional names, trimmed", () => {
    expect(normalizeModelList([{ id: " opus " }, { id: "gpt-5", name: " GPT-5 " }])).toEqual([
      { id: "opus" },
      { id: "gpt-5", name: "GPT-5" },
    ])
  })

  it("drops invalid entries, duplicates and names equal to the id", () => {
    expect(
      normalizeModelList([{ id: "" }, { name: "no id" }, "opus", null, { id: 3 }, { id: "a" }, { id: "a" }, { id: "b", name: "b" }]),
    ).toEqual([{ id: "a" }, { id: "b" }])
  })

  it("keeps the first entry of a duplicated id", () => {
    expect(
      normalizeModelList([{ id: "a", name: "First" }, { id: " a ", name: "Second" }, { id: "b" }, { id: "a" }]),
    ).toEqual([{ id: "a", name: "First" }, { id: "b" }])
  })
})

describe("buildCreateEngineModels", () => {
  it("omits engine_models when the selected engine has no model", () => {
    expect(buildCreateEngineModels("pi", {})).toBeUndefined()
    expect(buildCreateEngineModels("pi", { pi: "  ", claude: "opus" })).toBeUndefined()
    expect(buildCreateEngineModels("", { "": "x", pi: "a/b" })).toBeUndefined()
  })

  it("sends only the trimmed model of the selected engine", () => {
    expect(buildCreateEngineModels("pi", { pi: " a/b ", claude: "opus", codex: "gpt-5" })).toEqual({ pi: "a/b" })
  })

  it("drops models typed for engines the user switched away from", () => {
    expect(buildCreateEngineModels("codex", { pi: "a/b", claude: "opus" })).toBeUndefined()
  })
})

describe("changedEngineModels", () => {
  it("returns null when nothing changed", () => {
    expect(changedEngineModels({}, {})).toBeNull()
    expect(changedEngineModels({ pi: "a/b" }, { pi: "a/b" })).toBeNull()
    expect(changedEngineModels({ pi: "a/b" }, {})).toBeNull()
    expect(changedEngineModels({}, { pi: "" })).toBeNull()
  })

  it("returns new and changed models", () => {
    expect(changedEngineModels({ pi: "a/b" }, { pi: "a/c", claude: "opus" })).toEqual({ pi: "a/c", claude: "opus" })
  })

  it("returns an empty string for a cleared model", () => {
    expect(changedEngineModels({ pi: "a/b", claude: "opus" }, { pi: " ", claude: "opus" })).toEqual({ pi: "" })
  })
})

describe("engineModel", () => {
  it("reads the trimmed model of an engine", () => {
    expect(engineModel({ pi: " a/b " }, "pi")).toBe("a/b")
    expect(engineModel({ pi: "a/b" }, "claude")).toBe("")
    expect(engineModel(undefined, "pi")).toBe("")
    expect(engineModel({ "": "x" }, "")).toBe("")
  })
})

describe("filterModels", () => {
  it("returns every model for an empty query", () => {
    expect(filterModels(MODELS, "")).toEqual(MODELS)
    expect(filterModels(MODELS, "   ")).toEqual(MODELS)
  })

  it("matches the id or the name, ignoring case", () => {
    expect(filterModels(MODELS, "PRO").map((m) => m.id)).toEqual(["deepseek/deepseek-pro"])
    expect(filterModels(MODELS, "flash").map((m) => m.id)).toEqual(["deepseek/deepseek-flash"])
    expect(filterModels(MODELS, "gpt-5").map((m) => m.id)).toEqual(["openai/gpt-5"])
    expect(filterModels(MODELS, "deepseek/").map((m) => m.id)).toEqual(["deepseek/deepseek-flash", "deepseek/deepseek-pro"])
  })

  it("returns nothing when no model matches", () => {
    expect(filterModels(MODELS, "my-local-model")).toEqual([])
  })
})

describe("modelListNotice", () => {
  it("reports loading and errors before anything else", () => {
    expect(modelListNotice("loading", 0, 0)).toBe("loading")
    expect(modelListNotice("error", 0, 0)).toBe("error")
  })

  it("reports no matches only when a loaded list filters to nothing", () => {
    expect(modelListNotice("ready", 3, 0)).toBe("noMatches")
    expect(modelListNotice("ready", 3, 1)).toBeNull()
    expect(modelListNotice("ready", 0, 0)).toBeNull()
  })
})

describe("suggestionReducer", () => {
  it("opens with the full list and nothing highlighted", () => {
    const state = run([{ type: "open" }])
    expect(state).toEqual({ open: true, query: null, active: -1 })
    expect(visibleModels(MODELS, state)).toEqual(MODELS)
  })

  it("keeps the full list on reopen even when the field has a value", () => {
    const state = run([{ type: "input", text: "flash" }, { type: "close" }, { type: "open" }])
    expect(visibleModels(MODELS, state)).toEqual(MODELS)
  })

  it("filters by typed text and keeps the free text unpicked", () => {
    const state = run([{ type: "open" }, { type: "move", step: 1, count: 3 }, { type: "input", text: "my-model" }])
    expect(state).toEqual({ open: true, query: "my-model", active: -1 })
    expect(visibleModels(MODELS, state)).toEqual([])
    expect(highlightedModel(visibleModels(MODELS, state), state)).toBeUndefined()
  })

  it("opens on arrow keys when closed without highlighting", () => {
    expect(run([{ type: "move", step: 1, count: 3 }])).toEqual({ open: true, query: null, active: -1 })
  })

  it("moves down and up with wrap-around", () => {
    const down = run([{ type: "open" }, { type: "move", step: 1, count: 3 }])
    expect(down.active).toBe(0)
    expect(run([{ type: "move", step: 1, count: 3 }, { type: "move", step: 1, count: 3 }], down).active).toBe(2)
    expect(run([{ type: "move", step: 1, count: 3 }, { type: "move", step: 1, count: 3 }, { type: "move", step: 1, count: 3 }], down).active).toBe(0)
    expect(run([{ type: "move", step: -1, count: 3 }], down).active).toBe(2)
    expect(run([{ type: "open" }, { type: "move", step: -1, count: 3 }]).active).toBe(2)
  })

  it("restarts from the edge when the highlight is past a shorter list", () => {
    const state = run([{ type: "open" }, { type: "hover", index: 5 }, { type: "move", step: 1, count: 2 }])
    expect(state.active).toBe(0)
  })

  it("ignores moves when nothing is listed", () => {
    const state = run([{ type: "open" }, { type: "move", step: 1, count: 0 }])
    expect(state.active).toBe(-1)
  })

  it("highlights the hovered option and picks it", () => {
    const state = run([{ type: "open" }, { type: "hover", index: 2 }])
    expect(highlightedModel(visibleModels(MODELS, state), state)).toEqual(MODELS[2])
  })

  it("picks the highlighted option among the filtered ones", () => {
    const state = run([{ type: "open" }, { type: "input", text: "deepseek" }, { type: "move", step: 1, count: 2 }, { type: "move", step: 1, count: 2 }])
    expect(highlightedModel(visibleModels(MODELS, state), state)).toEqual(MODELS[1])
  })

  it("closes and forgets the highlight", () => {
    const state = run([{ type: "open" }, { type: "move", step: 1, count: 3 }, { type: "close" }])
    expect(state).toEqual(CLOSED_SUGGESTIONS)
    expect(highlightedModel(MODELS, state)).toBeUndefined()
  })

  it("ignores hover while closed", () => {
    expect(run([{ type: "hover", index: 1 }])).toBe(CLOSED_SUGGESTIONS)
  })
})

describe("parseEngineModels", () => {
  it("reads trimmed, non-empty models per engine", () => {
    expect(parseEngineModels('{"pi":" deepseek/deepseek-flash ","claude":"","codex":3}')).toEqual({
      pi: "deepseek/deepseek-flash",
    })
  })

  it("is empty for missing or unreadable values", () => {
    expect(parseEngineModels(undefined)).toEqual({})
    expect(parseEngineModels("")).toEqual({})
    expect(parseEngineModels("{")).toEqual({})
    expect(parseEngineModels('["pi"]')).toEqual({})
  })
})

describe("cleanEngineModels", () => {
  it("trims models and drops empty ones", () => {
    expect(cleanEngineModels({ pi: " a/b ", claude: " ", codex: "o3" })).toEqual({ pi: "a/b", codex: "o3" })
  })
})
