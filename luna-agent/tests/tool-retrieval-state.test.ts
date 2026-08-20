import { describe, expect, it } from "vitest"
import { ToolRetrievalStateTracker } from "../src/executor/retrieval-state.js"

describe("ToolRetrievalStateTracker", () => {
  it("keeps stable workflow facts and excludes page resource identifiers", () => {
    const tracker = new ToolRetrievalStateTracker({
      pageKind: "application-detail",
      resourceType: "Application",
      projectId: "project-secret-id",
      applicationId: "application-secret-id",
    })
    tracker.record("createRelease", "succeeded", {
      id: "release-secret-id",
      lunaVerification: { status: "pending", state: "running" },
    })

    const state = tracker.snapshot()
    expect(state.resourceContext).toEqual(["application", "application-detail"])
    expect(state.completedOperations).toEqual(["createRelease"])
    expect(state.stableOutcomes).toEqual(["createRelease:succeeded"])
    expect(state.pendingState).toBe("async_terminal_check")
    expect(JSON.stringify(state)).not.toContain("secret-id")
  })

  it("restores stable errors and clears async pending state on a terminal readback", () => {
    const tracker = new ToolRetrievalStateTracker({}, [{
      itemId: "item-1",
      type: "tool_call",
      status: "completed",
      content: {
        operationId: "createRelease",
        status: "failed",
        errorCode: "release.image_invalid",
      },
    }])
    tracker.record("getRelease", "succeeded", { status: "succeeded" })

    expect(tracker.snapshot()).toMatchObject({
      completedOperations: ["createRelease", "getRelease"],
      stableErrorCodes: ["release.image_invalid"],
      stableOutcomes: ["createRelease:failed", "getRelease:succeeded"],
    })
    expect(tracker.snapshot().pendingState).toBeUndefined()
  })

  it.each(["lost", "timeout", "rejected", "unavailable", "ready", "active"])("clears async pending state on %s", (status) => {
    const tracker = new ToolRetrievalStateTracker({})
    tracker.record("createRelease", "succeeded", {
      lunaVerification: { status: "pending", state: "running" },
    })
    tracker.record("getRelease", "succeeded", { status })

    expect(tracker.snapshot().pendingState).toBeUndefined()
  })

  it("bounds and normalizes stable error codes before retrieval", () => {
    const tracker = new ToolRetrievalStateTracker({})
    tracker.record("getRelease", "failed", undefined, ` RELEASE.INVALID_${"X".repeat(300)} `)

    expect(tracker.snapshot().stableErrorCodes).toHaveLength(1)
    expect(tracker.snapshot().stableErrorCodes[0]).toHaveLength(160)
    expect(tracker.snapshot().stableErrorCodes[0]).toMatch(/^release\.invalid_x+$/)
  })
})
