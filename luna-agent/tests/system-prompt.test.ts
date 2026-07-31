import { describe, expect, it } from "vitest"
import {
  loadedInteractionSkill,
  loadedNavigationSkill,
  loadedSkillReferences,
  systemPromptFor,
} from "../src/prompt/system.js"

describe("versioned system prompt", () => {
  it("loads concise interaction and navigation skill roots into system-v4", () => {
    const interaction = loadedInteractionSkill()
    const navigation = loadedNavigationSkill()
    const prompt = systemPromptFor("system-v4")

    expect(interaction).toContain("name: luna-devops-interaction")
    expect(interaction).toContain("使用 `send_message` 回答已知候选中的单击选择")
    expect(interaction).toContain("references/runtime-deployment.md")
    expect(navigation).toContain("name: luna-devops-navigation")
    expect(navigation).toContain("[标签](/已注册路径)")
    expect(prompt).toContain("<LUNA_DEVOPS_INTERACTION_SKILL>")
    expect(prompt).toContain("<LUNA_DEVOPS_NAVIGATION_SKILL>")
    expect(prompt).toContain(interaction)
    expect(prompt).toContain(navigation)
    expect(prompt).not.toContain("<LUNA_DEVOPS_REFERENCE")
  })

  it("loads deployment and project references for a deployment target choice", () => {
    const context = {
      userInput: "把 PostgreSQL 部署到哪个项目空间？",
      pageContext: { pathname: "/app-templates", routeName: "appTemplates" },
      operationIds: ["listProjects", "listRuntimeClusters"],
    }
    const names = loadedSkillReferences(context).map(item => item.name)
    const prompt = systemPromptFor("system-v4", context)

    expect(names).toContain("projects-applications")
    expect(names).toContain("runtime-deployment")
    expect(names).toContain("options-and-continuity")
    expect(names).not.toContain("routes")
    expect(prompt).toContain("使用 `send_message` 展示目标选项")
    expect(prompt).not.toContain("# 已注册的 Luna DevOps 路由")
  })

  it("loads routes only for navigation intent", () => {
    const inspectContext = {
      userInput: "打开轻雪项目空间并查看应用",
      operationIds: ["listProjects", "listApplications"],
    }
    const operationContext = {
      userInput: "把应用部署到轻雪项目空间",
      operationIds: ["listProjects", "listApplications"],
    }

    expect(loadedSkillReferences(inspectContext).map(item => item.name)).toContain("routes")
    expect(systemPromptFor("system-v4", inspectContext)).toContain("/projects/:projectId/apps/:applicationId")
    expect(loadedSkillReferences(operationContext).map(item => item.name)).not.toContain("routes")
  })

  it("loads diagnostics and security guidance from task and capability signals", () => {
    const names = loadedSkillReferences({
      userInput: "为什么这次操作失败且看不到原因？",
      pageContext: { pathname: "/settings/users" },
      operationIds: ["listPlatformEvents"],
    }).map(item => item.name)

    expect(names).toContain("diagnostics-observability")
    expect(names).toContain("security-administration")
    expect(names).toContain("options-and-continuity")
  })

  it("loads onboarding option guidance for users unfamiliar with the platform", () => {
    const context = {
      userInput: "你可以干什么，我应该怎么开始",
      pageContext: { pathname: "/dashboard", routeName: "dashboard" },
    }
    const names = loadedSkillReferences(context).map(item => item.name)
    const prompt = systemPromptFor("system-v4", context)

    expect(names).toContain("options-and-continuity")
    expect(prompt).toContain("必须调用 create_options 提供 2～5 个可直接点选的具体目标")
    expect(prompt).toContain("优先使用 `send_message`")
    expect(prompt).toContain("如果用户已经提出明确任务，应直接完成该任务")
  })

  it("requires an interaction form when an operation needs structured user input", () => {
    const context = {
      userInput: "帮我创建一个项目空间",
      pageContext: { pathname: "/projects", routeName: "projects" },
      operationIds: ["createProject"],
    }
    const names = loadedSkillReferences(context).map(item => item.name)
    const prompt = systemPromptFor("system-v4", context)

    expect(names).toContain("projects-applications")
    expect(names).toContain("card-templates")
    expect(prompt).toContain("就必须使用 create_interaction_cards")
    expect(prompt).toContain("一轮可完成时使用 form")
    expect(prompt).toContain("即使只缺一个结构化操作参数，也使用表单")
    expect(prompt).not.toContain("缺少参数时先用 send_message 收集")
  })

  it("does not load every domain merely because the full tool catalog is available", () => {
    const names = loadedSkillReferences({
      userInput: "帮我看看最近为什么构建失败",
      pageContext: { pathname: "/dashboard" },
      operationIds: [
        "listProjects",
        "listApplications",
        "listBuildRuns",
        "listReleases",
        "listRuntimeClusters",
        "listGatewayRoutes",
        "listPlatformEvents",
        "listUsers",
      ],
    }).map(item => item.name)

    expect(names).toContain("source-build-release")
    expect(names).toContain("diagnostics-observability")
    expect(names).not.toContain("runtime-deployment")
    expect(names).not.toContain("gateway-networking")
    expect(names).not.toContain("security-administration")
  })

  it("treats page context as guidance rather than authorization", () => {
    const prompt = systemPromptFor("system-v4")
    expect(prompt).toContain("页面上下文和会话上下文只用于帮助理解任务")
    expect(prompt).toContain("不是授权凭证或权限边界")
  })

  it("requires the card preparation handshake before the final card tool", () => {
    const prompt = systemPromptFor("system-v4")
    expect(prompt).toContain("先调用 prepare_interaction_cards")
    expect(prompt).toContain("完全相同的 generationId")
    expect(prompt).toContain("不得输出 HTML、CSS 或脚本")
  })

  it("keeps the complete model-facing prompt in Chinese", () => {
    const prompt = systemPromptFor("system-v4")
    expect(prompt).toContain("你是 Luna DevOps 的内嵌平台助手")
    expect(prompt).toContain("请使用以下交互 Skill")
    expect(prompt).not.toContain("You are Luna DevOps")
  })

  it("rejects obsolete prompt versions", () => {
    expect(() => systemPromptFor("system-v3" as never)).toThrow("ai.prompt_version_unavailable")
  })
})
