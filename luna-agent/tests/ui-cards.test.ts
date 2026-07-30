import { describe, expect, it } from "vitest"
import { createInteractionCardsInput, createInteractionCardsTool, normalizeInteractionCardsInput } from "../src/tools/ui-cards.js"

const databaseCard = {
  schemaVersion: 1,
  title: "选择数据库",
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
  it("accepts a catalog card with content, input and a tool action", () => {
    expect(createInteractionCardsInput.parse(databaseCard)).toMatchObject({
      schemaVersion: 1,
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
  })
})
