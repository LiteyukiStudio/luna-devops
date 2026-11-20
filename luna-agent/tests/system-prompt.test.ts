import { describe, expect, it } from "vitest"
import { systemPromptFor } from "../src/prompt/system.js"

describe("versioned system prompt", () => {
  it("ships one deterministic Chinese prompt with the interaction and navigation guidance", () => {
    const prompt = systemPromptFor("system-v4")

    expect(prompt).toMatch(/^你是 Luna DevOps 的内嵌平台助手/)
    expect(prompt.match(/<LUNA_DEVOPS_INTERACTION_GUIDANCE>/g)).toHaveLength(1)
    expect(prompt.match(/<LUNA_DEVOPS_NAVIGATION_GUIDANCE>/g)).toHaveLength(1)
    expect(prompt).toContain("# Luna DevOps 交互与推进规程")
    expect(prompt).toContain("# Luna DevOps 站内导航规程")
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

  it("routes product knowledge questions to the bundled docs instead of memory", () => {
    const prompt = systemPromptFor("system-v4")

    for (const rule of ["search_docs", "list_docs", "read_doc", "不得凭印象补全变量名", "受控文档链接"])
      expect(prompt).toContain(rule)
  })

  it("indexes progressive domain skills via load_skill and removes legacy conflicting instructions", () => {
    const prompt = systemPromptFor("system-v4")

    expect(prompt).toContain("load_skill")
    for (const skillId of [
      "workspace-iam",
      "source-registry-build",
      "application-deployment",
      "runtime-volumes",
      "gateway-topology",
      "diagnostics-governance",
      "platform-admin",
    ]) {
      expect(prompt).toContain(skillId)
    }
    for (const approvalRule of ["L0", "L1", "L2", "stage=prod"]) {
      expect(prompt).toContain(approvalRule)
    }
    for (const removedConflict of [
      "mode: presentation",
      "mode: interactive",
      "快捷选项",
      "[标签](/已注册路径)",
      "写后验收工具",
    ]) {
      expect(prompt).not.toContain(removedConflict)
    }
  })

  it("rejects obsolete prompt versions", () => {
    expect(() => systemPromptFor("system-v3" as never)).toThrow("ai.prompt_version_unavailable")
  })
})
