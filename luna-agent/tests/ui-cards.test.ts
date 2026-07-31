import { describe, expect, it } from "vitest"
import {
  createInteractionCardsInput,
  createInteractionCardsTool,
  normalizeInteractionCardsInput,
  prepareInteractionCardsInput,
  prepareInteractionCardsTool,
} from "../src/tools/ui-cards.js"

const databaseCard = {
  schemaVersion: 1,
  generationId: "database-candidates",
  title: "选择数据库",
  mode: "interactive",
  template: "catalog",
  cards: [{
    id: "postgresql",
    presentation: {
      variant: "application",
      title: "PostgreSQL",
      icon: { type: "category", name: "database", alt: "PostgreSQL" },
    },
    blocks: [{
      id: "facts",
      type: "key_value",
      items: [{ label: "版本", value: "16", format: "code" }],
    }],
    form: {
      sections: [{
        id: "target",
        fields: [{
          id: "projectId",
          type: "select",
          label: "项目空间",
          required: true,
          options: [{ value: "prj_example", label: "示例项目空间" }],
        }],
      }],
    },
    actions: [{
      id: "install",
      type: "tool",
      label: "安装",
      operationId: "installAppTemplate",
      bindings: [
        { target: "/projectId", value: { type: "field", fieldId: "projectId" } },
        { target: "/templateId", value: { type: "literal", value: "postgresql" } },
      ],
    }],
  }],
} as const

describe("interaction card tool", () => {
  it("publishes a bounded preparation tool linked by generation ID", () => {
    expect(prepareInteractionCardsInput.parse({
      schemaVersion: 1,
      generationId: "database-candidates",
      title: "正在整理数据库候选",
    })).toMatchObject({ generationId: "database-candidates" })
    expect(prepareInteractionCardsTool.operationId).toBe("prepare_interaction_cards")
  })

  it("accepts a catalog card with content, input and a tool action", () => {
    expect(createInteractionCardsInput.parse(databaseCard)).toMatchObject({
      schemaVersion: 1,
      generationId: "database-candidates",
      template: "catalog",
      cards: [{ id: "postgresql" }],
    })
  })

  it("rejects platform-only approval cards and duplicate card IDs", () => {
    expect(createInteractionCardsInput.safeParse({ ...databaseCard, template: "approval" }).success).toBe(false)
    expect(createInteractionCardsInput.safeParse({
      ...databaseCard,
      cards: [databaseCard.cards[0], databaseCard.cards[0]],
    }).success).toBe(false)
  })

  it("normalizes only representational version and missing section IDs", () => {
    const input = structuredClone(databaseCard) as unknown as Record<string, unknown>
    input.schemaVersion = "1"
    const cards = input.cards as Array<{ form: { sections: Array<Record<string, unknown>> } }>
    delete cards[0]!.form.sections[0]!.id

    const parsed = createInteractionCardsInput.parse(normalizeInteractionCardsInput(input))

    expect(parsed.schemaVersion).toBe(1)
    expect(parsed.cards[0]?.form?.sections[0]?.id).toBe("section_1_1")
  })

  it("allows message templates for public fields and rejects unknown or sensitive fields", () => {
    const input = structuredClone(databaseCard) as unknown as {
      cards: Array<{
        form: { sections: Array<{ fields: Array<Record<string, unknown>> }> }
        actions: Array<Record<string, unknown>>
      }>
    }
    input.cards[0]!.actions = [{
      id: "continue",
      type: "send_message",
      label: "继续",
      message: "继续处理 {{projectId}}",
    }]
    expect(createInteractionCardsInput.safeParse(input).success).toBe(true)

    input.cards[0]!.actions[0]!.message = "继续处理 {{missing}}"
    expect(createInteractionCardsInput.safeParse(input).success).toBe(false)

    input.cards[0]!.form.sections[0]!.fields.push({
      id: "password",
      type: "secret",
      label: "密码",
      generation: "optional",
    })
    input.cards[0]!.actions[0]!.message = "密码是 {{password}}"
    expect(createInteractionCardsInput.safeParse(input).success).toBe(false)
  })

  it("publishes the full generated JSON schema to the model", () => {
    const cards = (createInteractionCardsTool.inputSchema.properties as Record<string, Record<string, unknown>>).cards
    expect(cards).toBeDefined()
    if (!cards) throw new Error("create_interaction_cards cards schema is missing")
    expect(cards.maxItems).toBe(12)
    expect(JSON.stringify(cards)).toContain("presentation")
    expect(JSON.stringify(cards)).toContain("status_list")
    expect(JSON.stringify(cards)).toContain("multi_select")
    expect(createInteractionCardsTool.inputSchema.required).toContain("mode")
  })

  it.each([
    "catalog",
    "comparison",
    "inspector",
    "form",
    "wizard",
    "diagnosis",
    "plan",
    "progress",
    "result",
    "dashboard",
  ] as const)("accepts the %s workflow template", (template) => {
    const collectsInput = template === "form" || template === "wizard"
    expect(createInteractionCardsInput.safeParse({
      schemaVersion: 1,
      generationId: `${template}-fixture`,
      title: `${template} fixture`,
      mode: collectsInput ? "interactive" : "presentation",
      template,
      cards: [{
        id: `${template}-card`,
        presentation: { variant: "summary", title: `${template} card` },
        ...(collectsInput
          ? {
              form: {
                sections: [{
                  id: "main",
                  fields: [{ id: "target", type: "text", label: "Target", required: true }],
                }],
              },
              actions: [{
                id: "continue",
                type: "send_message",
                label: "Continue",
                message: "Continue with {{target}}",
              }],
            }
          : {}),
      }],
    }).success).toBe(true)
  })

  it("separates presentation cards from workflows waiting for user input", () => {
    expect(createInteractionCardsInput.safeParse({
      ...databaseCard,
      mode: "presentation",
    }).success).toBe(false)

    const displayOnlyCandidates = {
      schemaVersion: 1,
      generationId: "display-only-candidates",
      title: "请选择应用模板",
      mode: "interactive",
      template: "catalog",
      cards: [{
        id: "templates",
        presentation: { variant: "application", title: "应用模板市场" },
        blocks: [{
          id: "template-list",
          type: "item_list",
          items: [
            { id: "postgresql", primary: "PostgreSQL" },
            { id: "redis", primary: "Redis" },
          ],
        }],
      }],
    }
    expect(createInteractionCardsInput.safeParse(displayOnlyCandidates).success).toBe(false)

    expect(createInteractionCardsInput.safeParse({
      ...displayOnlyCandidates,
      template: "form",
      cards: [{
        id: "template-selection",
        presentation: { variant: "form", title: "选择应用模板" },
        form: {
          sections: [{
            id: "selection",
            fields: [{
              id: "templateId",
              type: "select",
              display: "radio",
              label: "应用模板",
              required: true,
              options: [
                { value: "postgresql", label: "PostgreSQL" },
                { value: "redis", label: "Redis" },
              ],
            }],
          }],
        },
        actions: [{
          id: "continue",
          type: "send_message",
          label: "继续配置",
          message: "继续配置 {{templateId}}。",
          emphasis: "primary",
        }],
      }],
    }).success).toBe(true)
  })

  it("rejects dangling source, relation, table, chart, progress and field references", () => {
    const invalidCases = [
      {
        block: { id: "source", type: "markdown", content: "content", sourceRefIds: ["missing"] },
      },
      {
        block: {
          id: "relations",
          type: "relations",
          nodes: [{ id: "known", label: "Known", category: "application" }],
          edges: [{ source: "known", target: "missing" }],
        },
      },
      {
        block: {
          id: "table",
          type: "data_table",
          columns: [{ key: "known", label: "Known" }],
          rows: [{ id: "row", cells: { unknown: "value" } }],
        },
      },
      {
        block: {
          id: "chart",
          type: "chart",
          chartType: "line",
          xAxis: ["a", "b"],
          series: [{ name: "value", values: [1] }],
        },
      },
      {
        block: { id: "progress", type: "progress", mode: "determinate", label: "Deploying" },
      },
    ]
    for (const invalidCase of invalidCases) {
      expect(createInteractionCardsInput.safeParse({
        schemaVersion: 1,
        generationId: "invalid-reference",
        title: "Invalid reference",
        mode: "presentation",
        template: "inspector",
        cards: [{
          id: "resource",
          presentation: { variant: "resource", title: "Resource" },
          blocks: [invalidCase.block],
        }],
      }).success).toBe(false)
    }

    expect(createInteractionCardsInput.safeParse({
      schemaVersion: 1,
      generationId: "invalid-field-reference",
      title: "Invalid field reference",
      mode: "interactive",
      template: "form",
      cards: [{
        id: "resource",
        presentation: { variant: "form", title: "Resource" },
        form: {
          sections: [{
            id: "main",
            fields: [{
              id: "dependent",
              type: "text",
              label: "Dependent",
              visibleWhen: { fieldId: "missing", operator: "equals", value: "yes" },
            }],
          }],
        },
      }],
    }).success).toBe(false)
  })

  it("accepts bounded maximum collections and rejects oversized groups", () => {
    const maximum = {
      schemaVersion: 1,
      generationId: "maximum",
      title: "Maximum",
      mode: "presentation",
      template: "comparison",
      cards: Array.from({ length: 12 }, (_, cardIndex) => ({
        id: `card-${cardIndex}`,
        presentation: { variant: "summary", title: `Card ${cardIndex}` },
        blocks: [{
          id: `table-${cardIndex}`,
          type: "data_table",
          columns: Array.from({ length: 8 }, (_, columnIndex) => ({ key: `column-${columnIndex}`, label: `Column ${columnIndex}` })),
          rows: Array.from({ length: 30 }, (_, rowIndex) => ({
            id: `row-${rowIndex}`,
            cells: Object.fromEntries(Array.from({ length: 8 }, (_, columnIndex) => [`column-${columnIndex}`, `${rowIndex}:${columnIndex}`])),
          })),
        }],
      })),
    }
    expect(createInteractionCardsInput.safeParse(maximum).success).toBe(true)
    expect(createInteractionCardsInput.safeParse({ ...maximum, cards: [...maximum.cards, maximum.cards[0]] }).success).toBe(false)
  })
})
