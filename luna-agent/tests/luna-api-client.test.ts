import { afterEach, describe, expect, it, vi } from "vitest"
import { hashCanonicalJSON } from "../src/canonical-json.js"
import { HttpLunaApiToolClient } from "../src/tools/luna-api-client.js"

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("Luna API tool client", () => {
  it("executes with the same canonical arguments that were bound to approval", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({ accessToken: "delegation" }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ accepted: true }), { status: 201 }))
    vi.stubGlobal("fetch", fetchMock)
    const argumentsValue = {
      templateId: "postgresql",
      body: { values: { username: "app", password: "generated" }, applicationName: "PostgreSQL" },
      projectId: "prj_1",
    }

    const result = await new HttpLunaApiToolClient("http://api:8080", "service-token").execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      operation: {
        operationId: "installAppTemplate",
        method: "POST",
        path: "/api/v1/projects/{projectId}/app-templates/{templateId}/install",
        category: "application",
        risk: "sensitive",
        requiredScopes: ["application:write"],
        approval: "always",
        idempotent: true,
        timeoutMs: 15000,
        inputSchema: {
          type: "object",
          properties: {
            projectId: { type: "string" },
            templateId: { type: "string" },
            body: { type: "object", additionalProperties: true },
          },
          required: ["projectId", "templateId", "body"],
          additionalProperties: false,
        },
      },
      arguments: argumentsValue,
      argumentsHash: hashCanonicalJSON(argumentsValue),
      runActorGrant: "grant",
      approvalGranted: true,
    })

    expect(result.status).toBe(201)
    const rawExecutionBody = (fetchMock.mock.calls[1]?.[1] as RequestInit).body
    expect(typeof rawExecutionBody).toBe("string")
    if (typeof rawExecutionBody !== "string")
      throw new TypeError("expected a string request body")
    const executionBody = JSON.parse(rawExecutionBody) as { argumentsCanonical: string }
    expect(hashCanonicalJSON(JSON.parse(executionBody.argumentsCanonical))).toBe(hashCanonicalJSON(argumentsValue))
    expect(executionBody.argumentsCanonical).toContain("generated")
  })

  it("retries transient failures only for idempotent tool execution", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({ accessToken: "delegation" }), { status: 200 }))
      .mockResolvedValueOnce(new Response("busy", { status: 503, headers: { "retry-after": "0" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ items: [] }), { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    const result = await new HttpLunaApiToolClient("http://api:8080", "service-token", 5).execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      operation: {
        operationId: "listProjects", method: "GET", path: "/api/v1/projects", category: "project",
        risk: "safe", requiredScopes: ["project:read"], approval: "never", idempotent: true, timeoutMs: 15000,
        inputSchema: { type: "object", properties: {}, additionalProperties: false },
      },
      arguments: {}, argumentsHash: hashCanonicalJSON({}), runActorGrant: "grant", approvalGranted: false,
    })
    expect(result.status).toBe(200)
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })

  it("does not retry a non-idempotent write with an unknown outcome", async () => {
    const fetchMock = vi.fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(JSON.stringify({ accessToken: "delegation" }), { status: 200 }))
      .mockResolvedValueOnce(new Response("busy", { status: 503 }))
    vi.stubGlobal("fetch", fetchMock)
    const result = await new HttpLunaApiToolClient("http://api:8080", "service-token", 5).execute({
      runId: "airun_1",
      toolCallId: "aitool_1",
      operation: {
        operationId: "createThing", method: "POST", path: "/api/v1/things", category: "application",
        risk: "sensitive", requiredScopes: ["application:write"], approval: "always", idempotent: false, timeoutMs: 15000,
        inputSchema: { type: "object", properties: {}, additionalProperties: false },
      },
      arguments: {}, argumentsHash: hashCanonicalJSON({}), runActorGrant: "grant", approvalGranted: true,
    })
    expect(result.status).toBe(503)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})
