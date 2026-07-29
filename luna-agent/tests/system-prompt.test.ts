import { describe, expect, it } from "vitest"
import { loadedNavigationSkill, systemPromptFor } from "../src/prompt/system.js"

describe("versioned system prompt", () => {
  it("loads the internal navigation skill into system-v3", () => {
    const skill = loadedNavigationSkill()
    const prompt = systemPromptFor("system-v3")

    expect(skill).toContain("name: luna-devops-navigation")
    expect(skill).toContain("[label](/registered/path)")
    expect(skill).toContain("/projects/:projectId/apps/:applicationId")
    expect(prompt).toContain("<LUNA_DEVOPS_NAVIGATION_SKILL>")
    expect(prompt).toContain(skill)
    expect(prompt).toContain("exactly one create_options")
    expect(prompt).toContain("2-5")
    expect(prompt).toContain("navigate_to_route")
    expect(prompt).toContain("Every option is independent")
  })

  it("keeps earlier prompt versions stable", () => {
    expect(systemPromptFor("system-v2")).not.toContain("LUNA_DEVOPS_NAVIGATION_SKILL")
    expect(systemPromptFor("system-v1")).toContain("read-only assistant")
  })
})
