import { describe, expect, it } from "vitest"
import { toolVisibility } from "../src/tools/tool-presentation.js"

describe("tool presentation policy", () => {
  it("marks maintenance-only tools as internal and business tools as normal", () => {
    expect(toolVisibility("create_options")).toBe("internal")
    expect(toolVisibility("rename_conversation")).toBe("internal")
    expect(toolVisibility("listProjects")).toBe("normal")
  })
})
