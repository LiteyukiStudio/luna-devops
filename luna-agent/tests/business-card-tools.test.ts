import { describe, expect, it } from "vitest"
import {
  businessCardToolInputs,
  businessCardTools,
  compileBusinessCardToolInput,
} from "../src/tools/business-card-tools.js"
import { createInteractionCardsInput } from "../src/tools/ui-cards.js"

const presentationCard = {
  schemaVersion: 1,
  title: "部署结果",
  mode: "presentation",
  template: "result",
  cards: [{
    id: "result",
    presentation: { variant: "receipt", title: "部署成功" },
    blocks: [{ id: "summary", type: "callout", tone: "success", content: "应用已通过健康检查。" }],
  }],
} as const

const inputCard = {
  schemaVersion: 1,
  title: "配置应用",
  mode: "interactive",
  placement: "turn_end",
  template: "form",
  cards: [{
    id: "configuration",
    presentation: { variant: "form", title: "应用配置" },
    form: {
      sections: [{
        id: "main",
        fields: [{ id: "name", type: "text", label: "应用名称", required: true }],
      }],
    },
    actions: [{ id: "submit", type: "send_message", label: "继续", message: "创建应用 {{name}}" }],
  }],
} as const

const choiceCard = {
  schemaVersion: 1,
  title: "选择项目空间",
  mode: "interactive",
  template: "candidates",
  cards: [
    {
      id: "prj_alpha",
      presentation: { variant: "resource", title: "Alpha" },
      actions: [{ id: "select", type: "send_message", label: "选择 Alpha", message: "使用 Alpha (prj_alpha)" }],
    },
    {
      id: "prj_beta",
      presentation: { variant: "resource", title: "Beta" },
      actions: [{ id: "select", type: "send_message", label: "选择 Beta", message: "使用 Beta (prj_beta)" }],
    },
  ],
} as const

describe("business card model tools", () => {
  it("registers only the three InteractionCardGroup v1 operations", () => {
    expect(businessCardTools.map(tool => tool.operationId)).toEqual([
      "present_card",
      "request_input",
      "request_choice",
    ])
    expect(Object.keys(businessCardToolInputs)).toEqual([
      "present_card",
      "request_input",
      "request_choice",
    ])
  })

  it("publishes object schemas for every registered operation", () => {
    for (const tool of businessCardTools)
      expect(tool.inputSchema).toMatchObject({ type: "object" })
  })

  it.each([
    ["present_card", presentationCard],
    ["request_input", inputCard],
    ["request_choice", choiceCard],
  ] as const)("accepts the current %s contract without a legacy compiler", (operationId, input) => {
    const compiled = compileBusinessCardToolInput(operationId, input)
    expect(compiled).toEqual(input)
    expect(createInteractionCardsInput.safeParse(compiled).success).toBe(true)
  })

  it("keeps each operation within its conversation responsibility", () => {
    expect(businessCardToolInputs.present_card.safeParse(inputCard).success).toBe(false)
    expect(businessCardToolInputs.request_input.safeParse(choiceCard).success).toBe(false)
    expect(businessCardToolInputs.request_choice.safeParse(inputCard).success).toBe(false)
  })

  it("rejects unknown root fields and model-provided Secret defaults", () => {
    expect(businessCardToolInputs.present_card.safeParse({
      ...presentationCard,
      generationId: "model-controlled",
    }).success).toBe(false)

    expect(businessCardToolInputs.request_input.safeParse({
      ...inputCard,
      cards: [{
        ...inputCard.cards[0],
        form: {
          sections: [{
            id: "secret",
            fields: [{
              id: "token",
              type: "secret",
              label: "访问令牌",
              generation: "disabled",
              defaultValue: "model-injected-token",
            }],
          }],
        },
      }],
    }).success).toBe(false)
  })
})
