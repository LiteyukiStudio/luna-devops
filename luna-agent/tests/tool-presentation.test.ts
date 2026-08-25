import { describe, expect, it } from "vitest"
import { businessCardToolInputs, businessCardToolOperationIds } from "../src/tools/business-card-tools.js"
import { toolVisibility } from "../src/tools/tool-presentation.js"

describe("tool presentation policy", () => {
  it("marks catalog and card protocol tools as internal and platform tools as normal", () => {
    expect(toolVisibility("search_tools")).toBe("internal")
    expect(toolVisibility("present_card")).toBe("internal")
    expect(toolVisibility("rename_conversation")).toBe("internal")
    expect(toolVisibility("navigate_to_route")).toBe("internal")
    expect(toolVisibility("listProjects")).toBe("normal")
    expect([...businessCardToolOperationIds].sort()).toEqual(Object.keys(businessCardToolInputs).sort())
  })
})
