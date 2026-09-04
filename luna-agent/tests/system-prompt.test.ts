import { describe, expect, it } from "vitest"
import { systemPromptFor } from "../src/prompt/system.js"

describe("versioned system prompt", () => {
  it("ships one deterministic Chinese prompt with the interaction and navigation skills", () => {
    const prompt = systemPromptFor("system-v4")

    expect(prompt).toMatch(/^你是 Luna DevOps 的内嵌平台助手/)
    expect(prompt.match(/<LUNA_DEVOPS_INTERACTION_SKILL>/g)).toHaveLength(1)
    expect(prompt.match(/<LUNA_DEVOPS_NAVIGATION_SKILL>/g)).toHaveLength(1)
    expect(prompt).toContain("name: luna-devops-interaction")
    expect(prompt).toContain("name: luna-devops-navigation")
    expect(prompt).not.toContain("You are Luna DevOps")
    expect(prompt).not.toContain("<LUNA_DEVOPS_REFERENCE")
  })

  it("retains the authorization, approval, secret, trust and completion boundaries", () => {
    const prompt = systemPromptFor("system-v4")

    for (const boundary of ["重新鉴权", "平台批准", "权威回读", "不可信数据", "Secret", "defaultValue"])
      expect(prompt).toContain(boundary)
  })

  it("keeps cross-tool delivery, dependency and diagnostic invariants", () => {
    const prompt = systemPromptFor("system-v4")

    for (const invariant of ["来源与配置", "构建或制品", "共享依赖", "独立凭据", "第一个异常边界", "沿原路径重新验收"])
      expect(prompt).toContain(invariant)
  })

  it("leaves operation-specific facts to the tool catalog", () => {
    const prompt = systemPromptFor("system-v4")

    expect(prompt).toContain("search_tools")
    expect(prompt).toContain("get_tool_details")
    for (const operationFact of ["listRuntimeClusters", "updateDeploymentTarget", "triggerBuildRun.targetRegistryId", "build.registry_push_credential_required"])
      expect(prompt).not.toContain(operationFact)
  })

  it("rejects obsolete prompt versions", () => {
    expect(() => systemPromptFor("system-v3" as never)).toThrow("ai.prompt_version_unavailable")
  })
})
