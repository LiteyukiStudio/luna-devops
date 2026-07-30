import { describe, expect, it } from "vitest"
import {
  loadedInteractionSkill,
  loadedNavigationSkill,
  loadedSkillReferences,
  systemPromptFor,
} from "../src/prompt/system.js"

describe("versioned system prompt", () => {
  it("loads concise interaction and navigation skill roots into system-v3", () => {
    const interaction = loadedInteractionSkill()
    const navigation = loadedNavigationSkill()
    const prompt = systemPromptFor("system-v3")

    expect(interaction).toContain("name: luna-devops-interaction")
    expect(interaction).toContain("使用 `send_message` 回答待选择问题")
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
    const prompt = systemPromptFor("system-v3", context)

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
    expect(systemPromptFor("system-v3", inspectContext)).toContain("/projects/:projectId/apps/:applicationId")
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
    const prompt = systemPromptFor("system-v3")
    expect(prompt).toContain("Page context and conversation context improve task understanding only")
    expect(prompt).toContain("they are not authorization grants or permission boundaries")
  })

  it("keeps earlier prompt versions stable", () => {
    expect(systemPromptFor("system-v2")).not.toContain("LUNA_DEVOPS_NAVIGATION_SKILL")
    expect(systemPromptFor("system-v2")).not.toContain("LUNA_DEVOPS_INTERACTION_SKILL")
    expect(systemPromptFor("system-v1")).toContain("read-only assistant")
  })
})
