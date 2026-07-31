import { describe, expect, it } from "vitest"
import { ToolCatalog } from "../src/tools/catalog.js"
import { platformOperations } from "../src/tools/generated/platform.js"

describe("platform tool catalog", () => {
  it("contains the BFF registered Gateway, Hook, notification, and runtime operations", () => {
    const catalog = ToolCatalog.load(platformOperations)
    expect([
      ["listGatewayRoutes", "gateway:read"],
      ["listGatewayCertificates", "gateway:read"],
      ["listProjectHookRuns", "project:read"],
      ["listNotificationDeliveries", "event:read"],
      ["listRuntimeEvents", "event:read"],
    ].map(([id]) => {
      const operation = catalog.get(id!)
      return [operation.operationId, operation.requiredScopes[0], operation.risk, operation.approval, operation.stepUpPurpose]
    })).toEqual([
      ["listGatewayRoutes", "gateway:read", "read", "never", undefined],
      ["listGatewayCertificates", "gateway:read", "read", "never", undefined],
      ["listProjectHookRuns", "project:read", "read", "never", undefined],
      ["listNotificationDeliveries", "event:read", "read", "never", undefined],
      ["listRuntimeEvents", "event:read", "read", "never", undefined],
    ])
  })

  it("offers project-scoped tools independently of page context and requires an explicit target", () => {
    const catalog = ToolCatalog.load(platformOperations)

    expect(catalog.modelTools().map(tool => tool.operationId)).toContain("listPlatformEvents")
    expect(catalog.get("listApplications").inputSchema.required).toEqual(["projectId"])
    expect(catalog.get("listProjects").inputSchema.properties).not.toHaveProperty("projectId")
  })

  it("exposes project creation as a user-authorized low-risk write without high-risk approval", () => {
    expect(ToolCatalog.load(platformOperations).get("createProject")).toMatchObject({
      requiredScopes: ["project:write"],
      risk: "write",
      approval: "never",
      idempotent: true,
    })
  })

  it("exposes bounded public web search and page reading as platform-scoped read tools", () => {
    const catalog = ToolCatalog.load(platformOperations)

    expect(catalog.get("webSearch")).toMatchObject({
      requiredScopes: ["web:read"],
      risk: "read",
      approval: "never",
      idempotent: true,
    })
    expect(catalog.get("fetchWebPage").inputSchema).toMatchObject({
      required: ["url"],
      additionalProperties: false,
      properties: {
        url: { type: "string", maxLength: 2048 },
        maxCharacters: { type: "integer", maximum: 50000 },
      },
    })
    expect(catalog.modelTools().find(tool => tool.operationId === "fetchWebPage")?.description)
      .toContain("不可信外部数据")
  })
})
