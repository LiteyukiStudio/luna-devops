import { describe, expect, it } from "vitest"
import {
  businessCardTools,
  compileBusinessCardToolInput,
  requestResourceChoiceInput,
  requestToolInputInput,
} from "../src/tools/business-card-tools.js"
import { createInteractionCardsInput, createInteractionCardsTool, normalizeInteractionCardsInput } from "../src/tools/ui-cards.js"

const toolInputFixtures = {
  request_resource_choice: {
    title: "选择项目空间",
    candidates: [
      { id: "prj_alpha", title: "Alpha" },
      { id: "prj_beta", title: "Beta" },
    ],
    selectionLabel: "使用这个项目空间",
    selectionMessage: "使用项目空间 {{candidate}}",
  },
  request_tool_input: {
    title: "配置应用",
    resourceTitle: "PostgreSQL 16",
    sections: [{
      id: "main",
      fields: [{ id: "name", type: "text", label: "应用名称", required: true, format: "identifier" }],
    }],
    submit: {
      type: "tool",
      label: "创建应用",
      operationId: "createApplication",
      fieldBindings: [{ target: "/body/name", fieldId: "name" }],
    },
  },
  review_tool_action: {
    title: "发布前核对",
    resourceTitle: "发布 api:v2",
    changes: [{ label: "镜像", value: "registry.example/api:v2", format: "code" }],
    submit: { type: "send_message", label: "继续发布", message: "按以上参数继续发布" },
  },
  present_diagnosis: {
    title: "构建失败诊断",
    conclusion: "Dockerfile 路径不存在。",
    conclusionTone: "error",
    findings: [{ id: "dockerfile", label: "Dockerfile", status: "error" }],
  },
  present_health_overview: {
    title: "应用健康状态",
    metrics: [{ label: "健康副本", value: "3/3", tone: "success" }],
    statuses: [{ id: "gateway", label: "访问入口", status: "success" }],
  },
  present_execution_progress: {
    title: "应用发布",
    binding: { operationType: "release", projectId: "prj_alpha", operationId: "rel_1" },
    label: "平台任务进度",
  },
  present_operation_result: {
    title: "发布结果",
    outcome: "success",
    summary: "应用已发布并通过权威健康检查。",
    facts: [{ label: "Release", value: "rel_1", format: "code" }],
  },
} as const

describe("narrow business card model tools", () => {
  it("exposes seven intent tools and keeps the full DSL out of the model-visible set", () => {
    expect(businessCardTools.map(tool => tool.operationId)).toEqual([
      "request_resource_choice",
      "request_tool_input",
      "review_tool_action",
      "present_diagnosis",
      "present_health_overview",
      "present_execution_progress",
      "present_operation_result",
    ])
    expect(businessCardTools.some(tool => tool.operationId === "create_interaction_cards")).toBe(false)
  })

  it("publishes strict object schemas that are substantially smaller than the legacy union", () => {
    const legacySchemaSize = JSON.stringify(createInteractionCardsTool.inputSchema).length
    for (const tool of businessCardTools) {
      expect(tool.inputSchema).toMatchObject({ type: "object", additionalProperties: false })
      expect(JSON.stringify(tool.inputSchema).length).toBeLessThan(legacySchemaSize * 0.25)
    }
  })

  it.each(Object.entries(toolInputFixtures))("compiles %s into a valid stable card contract", (operationId, raw) => {
    const compiled = compileBusinessCardToolInput(
      operationId as keyof typeof toolInputFixtures,
      raw,
    )
    expect(createInteractionCardsInput.safeParse(compiled).success).toBe(true)
  })

  it("selects candidate cards for short lists and a blocking select form for long lists", () => {
    const shortList = createInteractionCardsInput.parse(compileBusinessCardToolInput(
      "request_resource_choice",
      toolInputFixtures.request_resource_choice,
    ))
    expect(shortList).toMatchObject({ placement: "inline", mode: "interactive", template: "candidates" })
    expect(shortList.cards[0]?.actions?.[0]).toMatchObject({
      type: "send_message",
      message: "使用项目空间 Alpha (prj_alpha)",
    })

    const longList = createInteractionCardsInput.parse(compileBusinessCardToolInput("request_resource_choice", {
      ...toolInputFixtures.request_resource_choice,
      candidates: Array.from({ length: 6 }, (_, index) => ({ id: `prj_${index}`, title: `项目 ${index}` })),
    }))
    expect(longList).toMatchObject({ placement: "turn_end", mode: "interactive", template: "form" })
    expect(longList.cards[0]?.form?.sections[0]?.fields[0]).toMatchObject({
      type: "select",
      submissionFormat: "label_value",
    })
  })

  it("rejects unknown root fields and model-provided Secret defaults", () => {
    expect(requestResourceChoiceInput.safeParse({
      ...toolInputFixtures.request_resource_choice,
      schemaVersion: 1,
    }).success).toBe(false)
    expect(requestToolInputInput.safeParse({
      ...toolInputFixtures.request_tool_input,
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
    }).success).toBe(false)
  })

  it("continues to normalize a legacy persisted full-DSL input", () => {
    const legacy = {
      schemaVersion: "1",
      title: "历史诊断卡片",
      mode: "presentation",
      template: "result",
      cards: [{
        id: "legacy_result",
        presentation: { variant: "finding", title: "历史结论" },
        blocks: [{ id: "summary", type: "callout", tone: "warning", content: "需要继续观察。" }],
      }],
    }
    expect(createInteractionCardsInput.parse(normalizeInteractionCardsInput(legacy))).toMatchObject({
      schemaVersion: 1,
      cards: [{ id: "legacy_result" }],
    })
  })
})
