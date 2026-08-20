import { describe, expect, it } from "vitest"
import { createInteractionCardsInput, normalizeInteractionCardsInput } from "../src/tools/ui-cards.js"

function compile(businessTemplate: Record<string, unknown>) {
  return createInteractionCardsInput.parse(normalizeInteractionCardsInput({
    schemaVersion: 1,
    businessTemplate,
  }))
}

describe("business interaction card templates", () => {
  it("compiles rich short candidates into actionable cards", () => {
    const result = compile({
      templateId: "candidate_picker",
      title: "选择部署来源",
      candidates: [
        { id: "template", title: "应用市场模板", description: "使用预设配置", selectionLabel: "选择应用市场模板", selectionMessage: "使用应用市场模板" },
        { id: "image", title: "容器镜像", description: "直接发布已有镜像", selectionLabel: "选择容器镜像", selectionMessage: "使用容器镜像" },
      ],
    })
    expect(result).toMatchObject({ mode: "interactive", template: "candidates" })
    expect(result.cards).toHaveLength(2)
    expect(result.cards[0]?.actions?.[0]).toMatchObject({ type: "send_message", message: "使用应用市场模板" })
  })

  it("compiles long candidates into a bounded select form", () => {
    const result = compile({
      templateId: "candidate_select",
      title: "选择项目空间",
      fieldLabel: "项目空间",
      candidates: Array.from({ length: 6 }, (_, index) => ({ value: `prj_${index}`, label: `项目 ${index}` })),
      submitLabel: "确认项目空间",
      submitMessage: "使用项目空间 {{candidate}}",
    })
    expect(result).toMatchObject({ mode: "interactive", template: "form" })
    expect(result.cards[0]?.form?.sections[0]?.fields[0]).toMatchObject({ type: "select", id: "candidate", submissionFormat: "label_value" })
  })

  it("adds a creation entry alongside existing candidates", () => {
    const picker = compile({
      templateId: "candidate_picker",
      title: "选择项目空间",
      candidates: [
        { id: "a", title: "默认空间", selectionLabel: "使用默认空间", selectionMessage: "使用默认空间 (prj_a)" },
        { id: "b", title: "测试空间", selectionLabel: "使用测试空间", selectionMessage: "使用测试空间 (prj_b)" },
      ],
      creationAction: { label: "新建项目空间", message: "我想新建一个项目空间来部署应用" },
    })
    expect(picker.groupActions?.[0]).toMatchObject({ type: "send_message", label: "新建项目空间", emphasis: "secondary" })

    const select = compile({
      templateId: "candidate_select",
      title: "选择项目空间",
      fieldLabel: "项目空间",
      candidates: Array.from({ length: 6 }, (_, index) => ({ value: `prj_${index}`, label: `项目 ${index}` })),
      submitLabel: "确认项目空间",
      submitMessage: "使用项目空间 {{candidate}}",
      creationAction: { label: "新建项目空间", message: "我想新建一个项目空间" },
    })
    const actionTypes = select.cards[0]?.actions?.map(action => action.label)
    expect(actionTypes).toContain("新建项目空间")
  })

  it("compiles resource configuration and preserves tool bindings", () => {
    const result = compile({
      templateId: "resource_configuration",
      title: "配置应用",
      resourceTitle: "PostgreSQL 16",
      facts: [{ label: "项目空间", value: "prj_demo", format: "code" }],
      sections: [{ id: "main", fields: [{ id: "name", type: "text", label: "应用名称", required: true, format: "identifier" }] }],
      submit: {
        type: "tool",
        label: "创建应用",
        operationId: "createApplication",
        literalBindings: [{ target: "/projectId", value: "prj_demo" }],
        fieldBindings: [{ target: "/body/name", fieldId: "name" }],
      },
    })
    expect(result).toMatchObject({ mode: "interactive", template: "form" })
    expect(result.cards[0]?.actions?.[0]).toMatchObject({ type: "tool", operationId: "createApplication" })
  })

  it("rejects prototype-mutating JSON Pointer bindings", () => {
    expect(() => compile({
      templateId: "resource_configuration",
      title: "配置密钥",
      resourceTitle: "测试部署目标",
      sections: [{
        id: "main",
        fields: [{ id: "value", type: "secret", label: "密钥", generation: "disabled" }],
      }],
      submit: {
        type: "tool",
        label: "保存",
        operationId: "saveSecret",
        fieldBindings: [{ target: "/__proto__/polluted", fieldId: "value" }],
      },
    })).toThrow()
  })

  it("rejects JSON Pointer array indexes beyond the browser binding limit", () => {
    expect(() => compile({
      templateId: "resource_configuration",
      title: "配置密钥",
      resourceTitle: "测试部署目标",
      sections: [{
        id: "main",
        fields: [{ id: "value", type: "secret", label: "密钥", generation: "disabled" }],
      }],
      submit: {
        type: "tool",
        label: "保存",
        operationId: "saveSecret",
        fieldBindings: [{ target: "/body/items/1000/value", fieldId: "value" }],
      },
    })).toThrow()
  })

  it("rejects secret defaults through the business-template path", () => {
    const template = (field: Record<string, unknown>) => ({
      templateId: "resource_configuration",
      title: "配置应用",
      resourceTitle: "安全配置",
      sections: [{ id: "main", fields: [field] }],
      submit: {
        type: "tool",
        label: "保存",
        operationId: "saveConfig",
        fieldBindings: [{ target: "/value", fieldId: String(field.id) }],
      },
    })

    expect(() => compile(template({
      id: "password",
      type: "secret",
      label: "密码",
      generation: "disabled",
      defaultValue: "model-injected-password",
    }))).toThrow()
    expect(() => compile(template({
      id: "credentials",
      type: "key_value",
      label: "密钥变量",
      valueMode: "secret",
      defaultValue: [{ key: "TOKEN", value: "model-injected-token" }],
    }))).toThrow()
  })

  it("compiles a change review without pretending approval is complete", () => {
    const result = compile({
      templateId: "change_review",
      title: "发布前核对",
      resourceTitle: "发布 api:v2",
      changes: [{ label: "镜像", value: "registry.example/api:v2", format: "code" }],
      risks: [{ id: "availability", label: "可用性检查", status: "success" }],
      submit: { type: "send_message", label: "继续发布", message: "按以上参数继续发布" },
    })
    expect(result).toMatchObject({ mode: "interactive", template: "change_review" })
  })

  it.each([
    ["diagnosis_report", { conclusion: "构建失败源于 Dockerfile 路径不存在。", conclusionTone: "error", findings: [{ id: "dockerfile", label: "Dockerfile", status: "error" }] }, "result"],
    ["execution_progress", { binding: { operationType: "release", projectId: "prj_1", operationId: "rel_1" }, label: "正在发布" }, "live_task"],
    ["operation_result", { outcome: "success", summary: "应用已发布并通过健康检查。", facts: [{ label: "版本", value: "v2" }] }, "result"],
    ["health_overview", { metrics: [{ label: "健康副本", value: "3/3", tone: "success" }], statuses: [{ id: "gateway", label: "访问入口", status: "success" }] }, "result"],
  ] as const)("compiles the %s presentation template", (templateId, fields, expectedTemplate) => {
    const result = compile({ templateId, title: `${templateId} fixture`, ...fields })
    expect(result).toMatchObject({ mode: "presentation", template: expectedTemplate })
  })

  it("rejects candidate selection that cannot return the selected value", () => {
    expect(() => compile({
      templateId: "candidate_select",
      title: "选择项目空间",
      fieldLabel: "项目空间",
      candidates: Array.from({ length: 6 }, (_, index) => ({ value: `prj_${index}`, label: `项目 ${index}` })),
      submitLabel: "确认项目空间",
      submitMessage: "继续",
    })).toThrow()
  })
})
