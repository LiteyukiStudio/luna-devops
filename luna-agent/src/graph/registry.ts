import { Annotation, END, START, StateGraph } from "@langchain/langgraph"
import type { ModelProvider } from "../provider/provider.js"

const GraphState = Annotation.Root({
  input: Annotation<string>,
  pageContext: Annotation<Record<string, unknown>>,
  reasoningSummary: Annotation<string>,
  answer: Annotation<string>,
  toolCalls: Annotation<Array<{ operationId: string, arguments: Record<string, unknown> }>>,
})
export type AssistantGraphState = typeof GraphState.State
type CompiledAssistantGraph = {
  invoke(input: AssistantGraphState, options?: { signal?: AbortSignal }): Promise<AssistantGraphState>
}

export class GraphVersionRegistry {
  private readonly graphs = new Map<string, CompiledAssistantGraph>()
  constructor(provider: ModelProvider) {
    const graph = new StateGraph(GraphState)
      .addNode("context", state => ({ ...state, reasoningSummary: "正在检查会话上下文与可用的只读能力。" }))
      .addNode("respond", async state => {
        const response = await provider.complete({
          messages: [
            { role: "system", content: "You are Luna DevOps's read-only assistant. Never claim an action was executed. Do not reveal chain of thought or secrets." },
            { role: "user", content: `${state.input}\nPage context: ${JSON.stringify(state.pageContext)}` },
          ],
          maxOutputTokens: 1200,
        })
        return { ...state, answer: response.text, toolCalls: response.toolCalls ?? [], reasoningSummary: response.reasoningSummary ?? state.reasoningSummary }
      })
      .addEdge(START, "context").addEdge("context", "respond").addEdge("respond", END)
      .compile()
    this.graphs.set("assistant-v1", graph)
  }
  get(version: string) {
    const graph = this.graphs.get(version)
    if (!graph) throw new Error("ai.graph_version_unavailable")
    return graph
  }
  versions() { return [...this.graphs.keys()] }
}
