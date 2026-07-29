import { describe, expect, it } from "vitest"
import { ToolCatalog } from "../src/tools/catalog.js"
import { p2ReadonlyOperations } from "../src/tools/generated/p2-readonly.js"

describe("P2 read-only catalog", () => {
  it("contains the BFF registered Gateway, Hook, notification, and runtime operations", () => {
    const catalog = ToolCatalog.load(p2ReadonlyOperations)
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
})
