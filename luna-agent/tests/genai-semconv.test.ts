import { describe, expect, it } from "vitest"
import {
  configureGenAIAgentVersion,
  genAIAgentName,
  genAIAgentSpanAttributes,
  genAIClientTokenUsageAttributes,
  genAIInputMessages,
  genAIModelSpan,
  genAIOutputMessages,
  genAISchemaURL,
  genAIToolCallObject,
  genAIToolDefinitions,
  genAIToolSpanAttributes,
} from "../src/genai-semconv.js"

describe("OpenTelemetry GenAI semantic conventions", () => {
  it("uses the pinned development schema and canonical span attributes", () => {
    expect(genAISchemaURL).toBe("https://opentelemetry.io/schemas/gen-ai-dev/1.42.0-dev")
    expect(genAIAgentSpanAttributes("conversation-1", "gpt-5", 8192)).toMatchObject({
      "gen_ai.operation.name": "invoke_agent",
      "gen_ai.agent.name": genAIAgentName,
      "gen_ai.conversation.id": "conversation-1",
      "gen_ai.output.type": "text",
      "gen_ai.request.model": "gpt-5",
      "gen_ai.request.max_tokens": 8192,
    })
    expect(genAIModelSpan("https://models.example.com/v1", "openai", "gpt-5", 4096, true)).toEqual({
      name: "chat gpt-5",
      attributes: {
        "gen_ai.operation.name": "chat",
        "gen_ai.provider.name": "openai",
        "gen_ai.request.model": "gpt-5",
        "gen_ai.request.max_tokens": 4096,
        "gen_ai.output.type": "text",
        "gen_ai.request.stream": true,
        "server.address": "models.example.com",
        "server.port": 443,
      },
    })
    expect(genAIClientTokenUsageAttributes("deepseek", "deepseek-chat", "input", "deepseek-chat-v3")).toEqual({
      "gen_ai.operation.name": "chat",
      "gen_ai.provider.name": "deepseek",
      "gen_ai.request.model": "deepseek-chat",
      "gen_ai.response.model": "deepseek-chat-v3",
      "gen_ai.token.type": "input",
    })
    expect(genAIToolSpanAttributes({ name: "listProjects", callId: "call-1", description: "List projects" })).toEqual({
      "gen_ai.operation.name": "execute_tool",
      "gen_ai.agent.name": genAIAgentName,
      "gen_ai.tool.name": "listProjects",
      "gen_ai.tool.type": "extension",
      "gen_ai.tool.call.id": "call-1",
      "gen_ai.tool.description": "List projects",
    })
  })

  it("uses the explicitly configured Agent service version", () => {
    configureGenAIAgentVersion(" version-1 ")
    expect(genAIAgentSpanAttributes("conversation-1")).toMatchObject({ "gen_ai.agent.version": "version-1" })
    configureGenAIAgentVersion(undefined)
    expect(genAIAgentSpanAttributes("conversation-1")).not.toHaveProperty("gen_ai.agent.version")
  })

  it("emits model content in the official input and output message shapes", () => {
    expect(genAIInputMessages([
      { role: "system", content: "You are Luna" },
      { role: "user", content: "Deploy it" },
      { role: "assistant", content: "", toolCalls: [{ id: "call-1", operationId: "deployApplication", arguments: { deploymentId: "dep-1" } }] },
      { role: "tool", content: "{\"ok\":true}", toolCallId: "call-1" },
    ])).toEqual([
      { role: "system", parts: [{ type: "text", content: "You are Luna" }] },
      { role: "user", parts: [{ type: "text", content: "Deploy it" }] },
      { role: "assistant", parts: [{ type: "tool_call", id: "call-1", name: "deployApplication", arguments: { deploymentId: "dep-1" } }] },
      { role: "tool", parts: [{ type: "tool_call_response", id: "call-1", response: { ok: true } }] },
    ])
    expect(genAIOutputMessages({
      text: "Deployment started",
      reasoningSummary: "Validated configuration",
      toolCalls: [{ id: "call-1", operationId: "deployApplication", arguments: { deploymentId: "dep-1" } }],
      finishReason: "tool_call",
    })).toEqual([{
      role: "assistant",
      parts: [
        { type: "reasoning", content: "Validated configuration" },
        { type: "text", content: "Deployment started" },
        { type: "tool_call", id: "call-1", name: "deployApplication", arguments: { deploymentId: "dep-1" } },
      ],
      finish_reason: "tool_call",
    }])
  })

  it("emits draft-07 tool definitions and object tool payloads", () => {
    expect(genAIToolDefinitions([{
      operationId: "getProject",
      description: "Get a project",
      inputSchema: { $schema: "http://json-schema.org/draft-07/schema#", type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"] },
    }])).toEqual([{
      type: "function",
      name: "getProject",
      description: "Get a project",
      parameters: { $schema: "http://json-schema.org/draft-07/schema#", type: "object", properties: { projectId: { type: "string" } }, required: ["projectId"] },
    }])
    expect(genAIToolCallObject(["a", "b"])).toEqual({ items: ["a", "b"] })
    expect(genAIToolCallObject("ok")).toEqual({ value: "ok" })
  })
})
