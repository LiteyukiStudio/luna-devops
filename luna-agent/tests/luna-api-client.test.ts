import { afterEach, describe, expect, it, vi } from "vitest"
import { ToolCatalog, type ToolOperation } from "../src/tools/catalog.js"
import { buildToolRequest, HttpLunaApiToolClient } from "../src/tools/luna-api-client.js"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("Luna API tool client", () => {
  it("calls the real catalog route with path parameters, JSON body, and durable execution identity", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(new Response(JSON.stringify({ accepted: true }), { status: 201 }))
    vi.stubGlobal("fetch", fetchMock)
    const operation = toolOperation({
      operationId: "installAppTemplate",
      method: "POST",
      path: "/api/v1/projects/{projectId}/app-templates/{templateId}/install",
      parameters: [
        { inputName: "projectId", wireName: "projectId", in: "path", required: true },
        { inputName: "templateId", wireName: "templateId", in: "path", required: true },
      ],
      requestBody: true,
      requiresApproval: true,
    })
    const result = await new HttpLunaApiToolClient("http://api:8080", "service-token").execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      operation,
      arguments: { projectId: "prj_1", templateId: "postgresql", values: { username: "app", password: "generated" } },
    })

    expect(result.status).toBe(201)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toEqual(new URL("http://api:8080/api/v1/projects/prj_1/app-templates/postgresql/install"))
    expect(init?.headers).toMatchObject({
      authorization: "Bearer service-token",
      "x-luna-ai-run-id": "airun_1",
      "x-luna-ai-tool-call-id": "aitool_1",
      "idempotency-key": "aitool_1",
    })
    if (typeof init?.body !== "string") throw new Error("expected JSON request body")
    expect(JSON.parse(init.body)).toEqual({ values: { username: "app", password: "generated" } })
  })

  it("maps query parameters and retries transient failures only for idempotent operations", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response("busy", { status: 503, headers: { "retry-after": "0" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    const result = await new HttpLunaApiToolClient("http://api:8080", "service-token", 5).execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      operation: toolOperation({
        operationId: "listProjects",
        method: "GET",
        path: "/api/v1/projects",
        parameters: [{ inputName: "page", wireName: "page", in: "query", required: false }],
      }),
      arguments: { page: 2 },
    })
    expect(result.status).toBe(200)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(fetchMock.mock.calls[0]?.[0]).toEqual(new URL("http://api:8080/api/v1/projects?page=2"))
  })

  it("does not retry a non-idempotent write with an unknown outcome", async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValueOnce(new Response("busy", { status: 503 }))
    vi.stubGlobal("fetch", fetchMock)
    const result = await new HttpLunaApiToolClient("http://api:8080", "service-token", 5).execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      operation: toolOperation({ operationId: "createThing", method: "POST", path: "/api/v1/things", requestBody: true, idempotent: false }),
      arguments: { name: "thing" },
    })
    expect(result.status).toBe(503)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it("rejects catalog-defined headers so only the fixed execution identity crosses the boundary", () => {
    const operation = toolOperation({
      parameters: [{ inputName: "tenant", wireName: "X-Tenant", in: "header", required: true }],
    })

    expect(() => buildToolRequest(operation, { tenant: "untrusted" }, "http://api:8080"))
      .toThrow("ai.tool_catalog_invalid")
  })
})

function toolOperation(overrides: Partial<ToolOperation>): ToolOperation {
  return ToolCatalog.load([{
    operationId: "getThing",
    name: "测试工具",
    summary: "测试真实平台路由调用。",
    category: "test",
    requiredScopes: [],
    requiresApproval: false,
    idempotent: true,
    method: "GET",
    path: "/api/v1/things",
    inputSchema: { type: "object", properties: {}, required: [], additionalProperties: false },
    ...overrides,
  }]).all()[0]!
}
