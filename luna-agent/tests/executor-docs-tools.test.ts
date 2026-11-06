import { describe, expect, it } from "vitest"
import { DocsKnowledge } from "../src/knowledge/docs-knowledge.js"
import { ModelRuntime } from "../src/model-runtime.js"
import type { ModelMessage, ModelProvider, ModelToolCall } from "../src/provider/provider.js"
import { testRegistry } from "./support/model-tool-registry.js"
import { testExecutor } from "./support/run-executor.js"
import { TestRepository } from "./support/test-repository.js"

const usage = { status: "reported" as const, value: { inputTokens: 3, outputTokens: 2, totalTokens: 5 } }
const docs = new DocsKnowledge([
  { path: "zh/reference/image-template.md", content: "# 镜像模板变量\n\n镜像仓库路径模板支持变量。\n\n## 变量列表\n\n`{registryNamespace}` 镜像站命名空间；`{appIdentifier}` 应用标识。" },
], "test-digest", "https://docs.example.com", [])

function recordingProvider(steps: Array<{ text?: string, toolCalls?: ModelToolCall[] }>) {
  const requests: ModelMessage[][] = []
  const provider: ModelProvider = {
    async *stream(request) {
      requests.push(request.messages)
      const step = steps[Math.min(requests.length - 1, steps.length - 1)] ?? {}
      if (step.text) yield { type: "message_delta", delta: step.text }
      yield { type: "completed", usage, ...(step.toolCalls?.length ? { toolCalls: step.toolCalls } : {}) }
    },
    async complete() { return { text: "", toolCalls: [], usage } },
    capabilities: () => ({ streaming: true, toolCalling: true, structuredOutput: true }),
    health: async () => ({ ok: true }),
  }
  return { provider, requests }
}

async function queuedRun(owner: string, key: string) {
  const repository = new TestRepository()
  const conversation = await repository.createConversation(owner, key)
  const created = await repository.createTurn(owner, { conversationId: conversation.id, input: "目标镜像模板支持哪些变量？", pageContext: {}, idempotencyKey: key, actorSessionId: `ses_${key}` })
  return { repository, runId: created.run.id }
}

function toolResult(messages: ModelMessage[] | undefined, callId: string): Record<string, unknown> {
  const message = messages?.find(item => item.role === "tool" && item.toolCallId === callId)
  if (!message || message.role !== "tool") throw new Error(`missing tool result ${callId}`)
  // 执行层会给工具结果加上“不可信数据”前缀行，正文是 JSON。
  return JSON.parse(message.content.slice(message.content.indexOf("\n") + 1)) as Record<string, unknown>
}

describe("docs tools in the run loop", () => {
  it("searches, browses and reads bundled docs, then answers from them", async () => {
    const owner = "usr_docs_tools"
    const { repository, runId } = await queuedRun(owner, "docs-tools")
    const { provider, requests } = recordingProvider([
      { toolCalls: [{ id: "search-1", operationId: "search_docs", arguments: { query: "镜像模板 变量", locale: "zh" } }] },
      { toolCalls: [
        { id: "list-1", operationId: "list_docs", arguments: { locale: "zh", path: "reference" } },
        { id: "read-1", operationId: "read_doc", arguments: { docId: "zh/reference/image-template", sectionId: "变量列表" } },
      ] },
      { text: "支持 {registryNamespace} 和 {appIdentifier}。" },
    ])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry()), { docsKnowledge: docs }).runOnce()

    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    const search = toolResult(requests[1], "search-1")
    expect(search).toMatchObject({ status: "succeeded", docsVersion: "test-digest" })
    expect(search.items).toContainEqual(expect.objectContaining({ docId: "zh/reference/image-template", section: "变量列表", url: `https://docs.example.com/reference/image-template#${encodeURIComponent("变量列表")}` }))
    expect(toolResult(requests[2], "list-1")).toMatchObject({ status: "succeeded", entries: [{ type: "doc", title: "镜像模板变量", description: "镜像仓库路径模板支持变量。" }] })
    const read = toolResult(requests[2], "read-1")
    expect(read).toMatchObject({ status: "succeeded", title: "镜像模板变量", truncated: false })
    expect(String(read.content)).toContain("{registryNamespace}")
  })

  it("returns recoverable argument and not-found errors instead of failing the run", async () => {
    const owner = "usr_docs_tools_errors"
    const { repository, runId } = await queuedRun(owner, "docs-tools-errors")
    const { provider, requests } = recordingProvider([
      { toolCalls: [
        { id: "bad-1", operationId: "read_doc", arguments: {} },
        { id: "missing-1", operationId: "read_doc", arguments: { docId: "zh/does-not-exist" } },
        { id: "nodir-1", operationId: "list_docs", arguments: { locale: "zh", path: "guess/path" } },
      ] },
      { text: "当前文档未覆盖这个问题。" },
    ])

    await testExecutor(repository, new ModelRuntime(provider, testRegistry()), { docsKnowledge: docs }).runOnce()

    expect((await repository.getRun(owner, runId))?.status).toBe("completed")
    expect(toolResult(requests[1], "bad-1")).toMatchObject({ status: "failed", errorCode: "ai.tool_arguments_invalid", retryable: true })
    expect(toolResult(requests[1], "missing-1")).toMatchObject({ status: "not_found", errorCode: "ai.docs_not_found" })
    expect(toolResult(requests[1], "nodir-1")).toMatchObject({ status: "not_found", errorCode: "ai.docs_not_found" })
  })
})
