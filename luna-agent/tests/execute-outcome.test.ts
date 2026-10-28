import { describe, expect, it } from "vitest"
import { ModelRuntime } from "../src/model-runtime.js"
import type { ModelProvider } from "../src/provider/provider.js"
import { ToolCatalog } from "../src/tools/catalog.js"
import { DeterministicLunaApiClient } from "../src/tools/luna-api-client.js"
import { ProjectingToolCallStore, ToolOrchestrator } from "../src/tools/orchestrator.js"
import { toolOutcome } from "../src/tools/outcome.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testExecutor } from "./support/run-executor.js"
import { TestRepository } from "./support/test-repository.js"
import { testToolOperation } from "./support/tool-catalog.js"

const operation = testToolOperation("execReleaseRuntimeCommand", { method: "POST", effect: "execute", idempotent: false })

const provider: ModelProvider = {
  async *stream() { yield { type: "message_delta", delta: "unexpected model call" }; throw new Error("unexpected model call") },
  async complete() { throw new Error("unexpected model call") },
  capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
  health: async () => ({ ok: true }),
}

describe("command execution outcomes", () => {
  it.each([0, 42, undefined, "0"])("uses exitCode %s for durable tool and direct Run status", async (exitCode) => {
    const repository = new TestRepository()
    const conversation = await repository.createConversation("usr_exec", "execute")
    const created = await repository.createTurn("usr_exec", {
      conversationId: conversation.id, input: "执行命令", pageContext: { __lunaDirectToolAction: true },
      idempotencyKey: "execute", actorSessionId: "session_exec",
    })
    const catalog = ToolCatalog.load([operation])
    const client = new DeterministicLunaApiClient(() => ({ status: 200, body: { exitCode, stderr: "command output" } }))
    const tools = new ToolOrchestrator(catalog, client, new ProjectingToolCallStore(repository.toolCallStore, repository))
    const call = await tools.propose({ runId: created.run.id, operationId: operation.operationId, arguments: {}, inputMode: "direct" })
    expect(call.status).toBe(exitCode === 0 ? "succeeded" : "failed")
    const executor = testExecutor(repository, new ModelRuntime(provider, testRegistry()), { catalog, tools })
    await executor.runOnce()
    expect(await repository.getRun("usr_exec", created.run.id)).toMatchObject(exitCode === 0
      ? { status: "completed" }
      : { status: "failed", errorCode: "ai.tool_execution_failed" })
    expect(client.calls).toHaveLength(1)
  })

  it("does not require an exit code for data retention execution", () => {
    const call = {
      id: "call", runId: "run", operationId: "cleanupDataRetention", idempotencyKey: "key",
      status: "succeeded" as const, arguments: {}, argumentsHash: "hash", attempt: 1, executionCount: 1, rowVersion: 1,
      result: { deletedCount: 3 },
    }
    expect(toolOutcome(testToolOperation("cleanupDataRetention", { effect: "execute" }), call))
      .toMatchObject({ kind: "succeeded", verificationRequired: false })
  })
})
