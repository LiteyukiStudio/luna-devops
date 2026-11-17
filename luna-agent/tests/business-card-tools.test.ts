import { Buffer } from "node:buffer"
import { describe, expect, it } from "vitest"
import { CardGenerationService } from "../src/executor/cards.js"
import {
  businessCardToolOperationIds,
  businessCardToolInputs,
  businessCardTools,
  compileBusinessCardToolInput,
} from "../src/tools/business-card-tools.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { createInteractionCardsInput } from "../src/tools/ui-cards.js"
import { TestRepository } from "./support/test-repository.js"
import { testToolOperation } from "./support/tool-catalog.js"

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
    expect([...businessCardToolOperationIds].sort()).toEqual(Object.keys(businessCardToolInputs).sort())
  })

  it("physically shrinks and isolates exported JSON Schemas across the three tools", () => {
    const byOperation = new Map(businessCardTools.map(tool => [tool.operationId, JSON.stringify(tool.inputSchema)]))
    const presentJson = byOperation.get("present_card")!
    const inputJson = byOperation.get("request_input")!
    const choiceJson = byOperation.get("request_choice")!

    const presentBytes = Buffer.byteLength(presentJson, "utf8")
    const inputBytes = Buffer.byteLength(inputJson, "utf8")
    const choiceBytes = Buffer.byteLength(choiceJson, "utf8")
    const totalBytes = presentBytes + inputBytes + choiceBytes

    // 拆分前每个工具约 29,116 bytes（合计 87,348 bytes）；拆分后总体积下降超过 50%。
    expect(presentBytes).toBeLessThan(20_000)
    expect(inputBytes).toBeLessThan(17_000)
    expect(choiceBytes).toBeLessThan(9_000)
    expect(totalBytes).toBeLessThan(43_000)

    // present_card 彻底移除 form 与 9 种表单字段定义。
    expect(presentJson).not.toContain('"form"')
    expect(presentJson).not.toContain('"secret"')
    expect(presentJson).not.toContain('"visibleWhen"')
    expect(presentJson).not.toContain('"multi_select"')
    expect(presentJson).toContain('"live_progress"')
    expect(presentJson).toContain('"chart"')

    // request_input 彻底移除重型纯展示块。
    expect(inputJson).toContain('"form"')
    expect(inputJson).toContain('"secret"')
    expect(inputJson).not.toContain('"live_progress"')
    expect(inputJson).not.toContain('"chart"')
    expect(inputJson).not.toContain('"diff"')
    expect(inputJson).not.toContain('"relations"')
    expect(inputJson).not.toContain('"data_table"')

    // request_choice 彻底移除 form 块与重型纯展示块。
    expect(choiceJson).not.toContain('"form"')
    expect(choiceJson).not.toContain('"secret"')
    expect(choiceJson).not.toContain('"live_progress"')
    expect(choiceJson).not.toContain('"chart"')
    expect(choiceJson).not.toContain('"diff"')
    expect(choiceJson).not.toContain('"relations"')
  })

  it.each([
    ["present_card", presentationCard],
    ["request_input", inputCard],
    ["request_choice", choiceCard],
  ] as const)("normalizes %s inputs into InteractionCardGroup v1", (operationId, input) => {
    const compiled = compileBusinessCardToolInput(operationId, input)
    expect(compiled).toEqual(input)
    expect(createInteractionCardsInput.safeParse(compiled).success).toBe(true)
  })

  it("normalizes omitted constant fields and request_choice single/multi selection into InteractionCardGroup v1", () => {
    const minimalPresent = compileBusinessCardToolInput("present_card", {
      title: "变更预览",
      template: "change_review",
      cards: [{
        id: "review",
        presentation: { variant: "plan", title: "升级镜像版本" },
        blocks: [{ id: "diff", type: "diff", unifiedDiff: "- image: app:v1\n+ image: app:v2" }],
      }],
    })
    expect(minimalPresent).toMatchObject({
      schemaVersion: 1,
      mode: "presentation",
      template: "change_review",
    })

    const normalizedChoice = compileBusinessCardToolInput("request_choice", {
      title: "选择目标集群",
      cards: [{
        id: "clusters",
        presentation: { title: "可用集群" },
        selection: {
          id: "clusterIds",
          label: "集群列表",
          multiple: true,
          submissionFormat: "label_value",
          options: [
            { value: "clu_cq", label: "重庆集群" },
            { value: "clu_sh", label: "上海集群" },
          ],
        },
        actions: [{
          id: "confirm",
          type: "send_message",
          label: "确认",
          message: "部署到 {{clusterIds}}",
        }],
      }],
    })
    expect(normalizedChoice).toMatchObject({
      schemaVersion: 1,
      mode: "interactive",
      template: "candidates",
      cards: [{
        id: "clusters",
        form: {
          sections: [{
            fields: [{ id: "clusterIds", type: "multi_select" }],
          }],
        },
      }],
    })
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

  it("rejects hallucinated action.operationId values not present in ToolCatalog and enters repair", async () => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_card_catalog", "卡片校验")
    const turn = await repository.createTurn("usr_card_catalog", {
      conversationId: conversation.id,
      input: "收集输入",
      pageContext: {},
      idempotencyKey: "card-catalog-check",
    })
    const catalog = ToolCatalog.load([
      {
        ...testToolOperation("createApplication"),
        method: "POST",
        effect: "write",
        verification: [{ operationId: "getApplication", family: "application" }],
      },
      testToolOperation("getApplication"),
    ])
    const orchestrator = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: {} })),
      new MemoryToolCallStore(),
    )
    const cards = new CardGenerationService(
      repository,
      (runId, operationId) => orchestrator.isCallableOperation(runId, operationId),
    )

    const hallucinatedInput = compileBusinessCardToolInput("request_input", {
      title: "创建应用",
      cards: [{
        id: "create-app",
        presentation: { title: "创建应用" },
        form: {
          sections: [{
            id: "main",
            fields: [{ id: "appName", type: "text", label: "应用名", required: true }],
          }],
        },
        actions: [{
          id: "submit",
          type: "tool",
          label: "提交",
          operationId: "hallucinatedNonExistentTool",
          bindings: [{ target: "/name", value: { type: "field", fieldId: "appName" } }],
        }],
      }],
    })
    const generation = await cards.start(turn.run.id, turn.turn.id, hallucinatedInput, 2, "request_input", "call_card_1")
    const rejected = await cards.create(turn.run.id, hallucinatedInput, generation)
    expect(rejected.accepted).toBe(false)
    if (!rejected.accepted) {
      expect(rejected.failure.retryable).toBe(true)
      expect(rejected.failure.issues).toEqual([
        expect.objectContaining({
          code: "unknown_tool_operation",
          path: "cards.0.actions.0.operationId",
        }),
      ])
    }

    const repairedInput = compileBusinessCardToolInput("request_input", {
      title: "创建应用",
      cards: [{
        id: "create-app",
        presentation: { title: "创建应用" },
        form: {
          sections: [{
            id: "main",
            fields: [{ id: "appName", type: "text", label: "应用名", required: true }],
          }],
        },
        actions: [{
          id: "submit",
          type: "tool",
          label: "提交",
          operationId: "createApplication",
          bindings: [{ target: "/name", value: { type: "field", fieldId: "appName" } }],
        }],
      }],
    })
    const accepted = await cards.create(turn.run.id, repairedInput, generation)
    expect(accepted).toEqual({ accepted: true, mode: "interactive" })
  })
})
