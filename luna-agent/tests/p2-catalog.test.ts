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

  it("offers project-scoped tools only when the page context has a project", () => {
    const catalog = ToolCatalog.load(platformOperations)

    expect(catalog.modelTools().map(tool => tool.operationId)).toEqual(["getDashboard", "listProjects", "createProject"])
    expect(catalog.modelTools({ projectId: "project-1" }).map(tool => tool.operationId)).toContain("listPlatformEvents")
  })

  it("exposes project creation as a user-authorized low-risk write without high-risk approval", () => {
    expect(ToolCatalog.load(platformOperations).get("createProject")).toMatchObject({
      requiredScopes: ["project:write"],
      risk: "write",
      approval: "never",
      idempotent: true,
    })
  })
})
