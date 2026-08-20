import { describe, expect, it } from "vitest"
import { modelVisibleHistory } from "../src/model-history.js"

describe("model-visible conversation history", () => {
  it("restores the real narrow card operation without changing the timeline projection", () => {
    const history = [{
      turnIndex: 1,
      user: "选择资源",
      assistant: "请选择",
      toolInteractions: [{
        type: "tool_call",
        content: {
          operationId: "create_interaction_cards",
          modelOperationId: "request_resource_choice",
          arguments: { template: "form" },
        },
      }],
    }]

    expect(modelVisibleHistory(history)[0]?.toolInteractions?.[0]).toMatchObject({
      content: {
        operationId: "request_resource_choice",
        timelineOperationId: "create_interaction_cards",
      },
    })
    expect(history[0]?.toolInteractions?.[0]).toMatchObject({ content: { operationId: "create_interaction_cards" } })
  })
})
