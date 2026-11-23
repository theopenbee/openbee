import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { describe, expect, it } from "vitest"
import type { ModelListNotice } from "@/lib/engine-models"
import type { EngineModel } from "@/lib/types"
import { ModelCombobox, ModelHint, ModelSuggestions, type ModelNotices } from "./model-field"

const NOTICES: ModelNotices = {
  loading: "Loading models…",
  error: "Couldn't load the model list. You can still type a model.",
  noMatches: "No matching models. The typed name is used as is.",
}

const MODELS: EngineModel[] = [{ id: "deepseek/deepseek-flash", name: "DeepSeek Flash" }, { id: "opus" }]

const suggestions = (over: { models?: EngineModel[]; active?: number; selected?: string } = {}) =>
  renderToStaticMarkup(
    createElement(ModelSuggestions, {
      id: "m-listbox",
      label: "Model",
      models: over.models ?? MODELS,
      active: over.active ?? -1,
      selected: over.selected ?? "",
      onPick: () => {},
      onHover: () => {},
    }),
  )

const combobox = (value: string, disabled = false) =>
  renderToStaticMarkup(
    createElement(ModelCombobox, {
      id: "m",
      label: "Model",
      description: "Leave blank to use OpenCode's default model.",
      placeholder: "Engine default",
      value,
      onChange: () => {},
      models: MODELS,
      status: "ready",
      notices: NOTICES,
      disabled,
    }),
  )

describe("ModelCombobox", () => {
  it("renders a closed free-text combobox with the typed value", () => {
    const html = combobox("my/custom-model")
    expect(html).toContain('role="combobox"')
    expect(html).toContain('aria-autocomplete="list"')
    expect(html).toContain('aria-expanded="false"')
    expect(html).toContain('value="my/custom-model"')
    expect(html).not.toContain('role="listbox"')
    expect(html).not.toContain("aria-activedescendant")
  })

  it("shows the placeholder and helper for an empty value", () => {
    const html = combobox("")
    expect(html).toContain('placeholder="Engine default"')
    expect(html).toContain('aria-describedby="m-hint"')
    expect(html).toContain('id="m-hint"')
    expect(html).toContain("Leave blank to use OpenCode&#x27;s default model.")
  })

  it("can be disabled", () => {
    expect(combobox("", true)).toContain("disabled")
  })
})

describe("ModelSuggestions", () => {
  it("lists every model as an option with its display name", () => {
    const html = suggestions()
    expect(html).toContain('role="listbox"')
    expect(html).toContain('aria-label="Model"')
    expect(html.match(/role="option"/g)).toHaveLength(2)
    expect(html).toContain("deepseek/deepseek-flash")
    expect(html).toContain("DeepSeek Flash")
    expect(html).not.toContain('role="status"')
  })

  it("marks the highlighted option for aria-activedescendant", () => {
    const html = suggestions({ active: 1 })
    expect(html).toContain('id="m-listbox-1"')
    expect(html).toMatch(/id="m-listbox-1"[^>]*aria-selected="true"/)
    expect(html).toMatch(/id="m-listbox-0"[^>]*aria-selected="false"/)
  })

  it("checks the option that matches the current value", () => {
    expect(suggestions({ selected: "opus" })).toContain("lucide-check")
    expect(suggestions({ selected: "my/custom" })).not.toContain("lucide-check")
  })

  it("uses Arc radius tokens for the panel and options", () => {
    const html = suggestions()
    expect(html).toContain("rounded-panel")
    expect(html).toContain("rounded-control")
    expect(html).not.toMatch(/rounded-(sm|md|lg|xl|\[)/)
  })
})

const hint = (notice: ModelListNotice | null, description?: string) =>
  renderToStaticMarkup(createElement(ModelHint, { id: "m-hint", description, notice, notices: NOTICES }))

describe("ModelHint", () => {
  it("shows the helper text without a notice", () => {
    const html = hint(null, "Leave blank to use the default.")
    expect(html).toContain('id="m-hint"')
    expect(html).toContain("Leave blank to use the default.")
    expect(html).not.toContain('role="status"')
  })

  it("replaces the helper text when filtering finds nothing", () => {
    const html = hint("noMatches", "Leave blank to use the default.")
    expect(html).toContain('role="status"')
    expect(html).toContain("No matching models. The typed name is used as is.")
    expect(html).not.toContain("Leave blank")
  })

  it("shows the list error while still allowing free text", () => {
    expect(hint("error")).toContain("You can still type a model.")
  })

  it("shows a loading notice", () => {
    expect(hint("loading")).toContain("Loading models…")
  })

  it("renders nothing without helper text or notice", () => {
    expect(hint(null)).toBe("")
  })
})
