import { describe, expect, it } from "vitest"
import { ModelRuntime } from "../src/model-runtime.js"
import type { ModelProvider, ModelRequest, ModelToolCall } from "../src/provider/provider.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { MemoryToolCallStore, ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { domainSkillIds, listDomainSkills, loadDomainSkills, loadSkillTool } from "../src/tools/skill-loader.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testExecutor } from "./support/run-executor.js"
import { TestRepository } from "./support/test-repository.js"
import { testToolOperation } from "./support/tool-catalog.js"

const reportedUsage = { status: "reported" as const, value: { inputTokens: 20, outputTokens: 10, totalTokens: 30 } }

describe("progressive domain skills", () => {
  it("loads all 6 domain skills with Run-scoped deduplication and partial cache hits", () => {
    const index = listDomainSkills()
    expect(index.map(item => item.skillId)).toEqual([...domainSkillIds])
    for (const item of index) {
      expect(item.description.length).toBeGreaterThan(8)
    }

    const loadedSet = new Set<string>()
    const first = loadDomainSkills(["diagnostics-governance", "runtime-volumes"], loadedSet)
    expect(first.cacheHit).toBe(false)
    expect(first.loadedSkillIds).toEqual(["diagnostics-governance", "runtime-volumes"])
    expect(first.alreadyLoadedSkillIds).toEqual([])
    expect(first.items[0]?.instructions).toContain("getProjectDiagnostics")
    expect(first.items[1]?.instructions).toContain("unavailable")
    for (const id of first.loadedSkillIds) loadedSet.add(id)

    const duplicate = loadDomainSkills(["diagnostics-governance"], loadedSet)
    expect(duplicate.cacheHit).toBe(true)
    expect(duplicate.duplicate).toBe(true)
    expect(duplicate.loadedSkillIds).toEqual([])
    expect(duplicate.alreadyLoadedSkillIds).toEqual(["diagnostics-governance"])
    expect(duplicate.items[0]?.instructions).toBeUndefined()

    const mixed = loadDomainSkills(["diagnostics-governance", "gateway-topology"], loadedSet)
    expect(mixed.cacheHit).toBe(false)
    expect(mixed.loadedSkillIds).toEqual(["gateway-topology"])
    expect(mixed.alreadyLoadedSkillIds).toEqual(["diagnostics-governance"])
    expect(mixed.items.find(item => item.skillId === "diagnostics-governance")?.instructions).toBeUndefined()
    expect(mixed.items.find(item => item.skillId === "gateway-topology")?.instructions).toContain("GatewayRoute")
  })

  it("executes load_skill inside RunExecutor, continues to the next model step, and preserves loadedSkillIds across approval resume", async () => {
    const repository = new TestRepository()
    const catalog = ToolCatalog.load([
      {
        ...testToolOperation("restartDeploymentTarget"),
        operationId: "restartDeploymentTarget",
        method: "POST",
        path: "/api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/restart",
        category: "deployment",
        approvalTier: "L2",
        requiresApproval: true,
        requiredScopes: ["deployment:restart"],
        effect: "write",
        verification: [{ operationId: "restartDeploymentTarget", family: "resource" }],
        inputSchema: {
          type: "object",
          required: ["projectId", "applicationId", "targetId"],
          properties: {
            projectId: { type: "string" },
            applicationId: { type: "string" },
            targetId: { type: "string" },
          },
          additionalProperties: false,
        },
      },
    ])
    const memoryStore = new MemoryToolCallStore()
    const projectingStore = new ProjectingToolCallStore(memoryStore, repository)
    const tools = new ToolOrchestrator(
      catalog,
      new DeterministicLunaApiClient(() => ({ status: 200, body: { restarted: true } })),
      projectingStore,
    )

    let step = 0
    let resumedDuplicateFlag: boolean | undefined
    const provider: ModelProvider = {
      async *stream(request: ModelRequest) {
        step += 1
        let toolCalls: ModelToolCall[] = []
        let text = ""
        if (step === 1) {
          toolCalls = [{ id: "call_skill_1", operationId: "load_skill", arguments: { skillIds: ["application-deployment"] } }]
        }
        else if (step === 2) {
          toolCalls = [{
            id: "call_restart_1",
            operationId: "restartDeploymentTarget",
            arguments: { projectId: "prj_1", applicationId: "app_1", targetId: "tgt_1" },
          }]
        }
        else if (step === 3) {
          toolCalls = [{ id: "call_skill_dup", operationId: "load_skill", arguments: { skillIds: ["application-deployment"] } }]
        }
        else {
          const lastToolMsg = [...request.messages].reverse().find(m => m.role === "tool" && m.toolCallId === "call_skill_dup")
          if (lastToolMsg && lastToolMsg.role === "tool") {
            resumedDuplicateFlag = /"cacheHit"\s*:\s*true/.test(lastToolMsg.content)
          }
          text = "已完成重启确认。"
        }
        if (text) yield { type: "message_delta" as const, delta: text }
        if (toolCalls.length) yield { type: "tool_call_delta" as const }
        yield {
          type: "completed" as const,
          usage: reportedUsage,
          ...(toolCalls.length ? { toolCalls } : {}),
        }
      },
      async complete() {
        return { text: "重启部署", toolCalls: [], usage: reportedUsage }
      },
      capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
      health: async () => ({ ok: true }),
    }

    const modelRuntime = new ModelRuntime(provider, testRegistry({
      resolve: () => [...catalog.modelTools(["restartDeploymentTarget"]), loadSkillTool],
    }))
    const executor = testExecutor(repository, modelRuntime, { tools, catalog })

    const conv = await repository.createConversation("usr_skill", "Skill Resume")
    const created = await repository.createTurn("usr_skill", {
      conversationId: conv.id,
      input: "重启部署",
      pageContext: {},
      idempotencyKey: "skill-resume-turn",
    })

    await executor.runOnce()
    expect((await repository.getRun("usr_skill", created.run.id))?.status).toBe("waiting_approval")

    const pending = [...memoryStore.records.values()][0]!
    await tools.recordApprovalDecision(pending.id, "approve")
    await repository.updateRun(created.run.id, "waiting_approval", "queued")

    await executor.runOnce()
    expect((await repository.getRun("usr_skill", created.run.id))?.status).toBe("completed")
    expect(resumedDuplicateFlag).toBe(true)
  })
})
