import { afterEach, describe, expect, it, vi } from "vitest"
import { ToolCatalog, type ToolOperation } from "../src/tools/catalog.js"
import { HttpLunaApiToolClient } from "../src/tools/luna-api-client.js"
import { testToolOperation } from "./support/tool-catalog.js"

afterEach(() => {
  vi.unstubAllGlobals()
})

function toolOperation(overrides: Partial<ToolOperation> & { operationId: string }): ToolOperation {
  const { operationId, ...rest } = overrides
  return ToolCatalog.load([testToolOperation(operationId, rest)]).get(operationId)
}

describe("Luna API tool client", () => {
  it("dispatches tool execution via unified POST /internal/v1/ai/tools/execute with preview lock", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(
      new Response(JSON.stringify({ accepted: true, projectId: "prj_1" }), { status: 200 }),
    )
    vi.stubGlobal("fetch", fetchMock)
    const operation = toolOperation({
      operationId: "delivery_install_template",
      approvalTier: "L2",
      requiresApproval: true,
    })
    const result = await new HttpLunaApiToolClient("http://api:8080", "service-token").execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      idempotencyKey: "aiidem_1",
      previewLock: "ailock_v1.payload.sig",
      operation,
      arguments: { project: "prj_1", template: "postgresql", secretValues: { password: "generated" } },
    })

    expect(result.status).toBe(200)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toEqual(new URL("http://api:8080/internal/v1/ai/tools/execute"))
    expect(init?.method).toBe("POST")
    expect(init?.headers).toMatchObject({
      "authorization": "Bearer service-token",
      "x-luna-ai-run-id": "airun_1",
      "x-luna-ai-tool-call-id": "aitool_1",
      "idempotency-key": "aiidem_1",
      "x-luna-ai-preview-lock": "ailock_v1.payload.sig",
      "content-type": "application/json",
    })
    expect(JSON.parse(typeof init?.body === "string" ? init.body : "")).toEqual({
      toolName: "delivery_install_template",
      arguments: { project: "prj_1", template: "postgresql", secretValues: { password: "generated" } },
    })
  })

  it("dispatches L2 pre-approval preview via POST /internal/v1/ai/tools/preview", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(
      new Response(JSON.stringify({
        approvalPreview: { summary: "Install PostgreSQL" },
        previewLock: "ailock_v1.test.sig",
      }), { status: 200 }),
    )
    vi.stubGlobal("fetch", fetchMock)
    const operation = toolOperation({
      operationId: "delivery_install_template",
      approvalTier: "L2",
      requiresApproval: true,
    })
    const result = await new HttpLunaApiToolClient("http://api:8080", "service-token").preview({
      runId: "airun_1",
      toolCallId: "aitool_1",
      operation,
      arguments: { project: "prj_1", template: "postgresql" },
    })

    expect(result.status).toBe(200)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toEqual(new URL("http://api:8080/internal/v1/ai/tools/preview"))
    expect(init?.method).toBe("POST")
    expect(JSON.parse(typeof init?.body === "string" ? init.body : "")).toEqual({
      toolName: "delivery_install_template",
      arguments: { project: "prj_1", template: "postgresql" },
    })
  })

  it("retries transient failures only for idempotent operations", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response("busy", { status: 503, headers: { "retry-after": "0" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    const result = await new HttpLunaApiToolClient("http://api:8080", "service-token", 5).execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      idempotencyKey: "aitool_1",
      operation: toolOperation({
        operationId: "workspace_list_projects",
        idempotent: true,
      }),
      arguments: { page: 2 },
    })
    expect(result.status).toBe(200)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it("does not retry a non-idempotent write with an unknown outcome", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(new Response("busy", { status: 503 }))
    vi.stubGlobal("fetch", fetchMock)
    const result = await new HttpLunaApiToolClient("http://api:8080", "service-token", 5).execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      idempotencyKey: "aitool_1",
      operation: toolOperation({ operationId: "delivery_create_application", idempotent: false }),
      arguments: { name: "thing" },
    })
    expect(result.status).toBe(503)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it("marks a successful response with an invalid JSON body as an unknown outcome", async () => {
    vi.stubGlobal("fetch", vi.fn<typeof fetch>().mockResolvedValueOnce(new Response("<html>upstream proxy error</html>", { status: 200 })))

    await expect(new HttpLunaApiToolClient("http://api:8080", "service-token").execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      idempotencyKey: "aitool_1",
      operation: toolOperation({ operationId: "workspace_get_project", idempotent: false }),
      arguments: {},
    })).rejects.toMatchObject({
      name: "UnknownToolOutcomeError",
      message: "ai.tool_outcome_unknown",
    })
  })

  it("marks a transport failure after dispatch as an unknown tool outcome", async () => {
    vi.stubGlobal("fetch", vi.fn<typeof fetch>().mockRejectedValueOnce(new Error("request timeout")))

    await expect(new HttpLunaApiToolClient("http://api:8080", "service-token").execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      idempotencyKey: "aitool_1",
      operation: toolOperation({ operationId: "build_trigger_run", idempotent: false }),
      arguments: { project: "prj_1" },
    })).rejects.toMatchObject({
      name: "UnknownToolOutcomeError",
      message: "ai.tool_outcome_unknown",
    })
  })
})
