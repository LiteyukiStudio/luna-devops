import { describe, expect, it } from "vitest"
import { canonicalJSONStringify, hashCanonicalJSON } from "../src/canonical-json.js"

describe("canonical tool arguments", () => {
  it("produces the same hash regardless of object insertion order", () => {
    const first = {
      projectId: "prj_1",
      templateId: "postgresql",
      body: {
        values: { username: "app", password: "generated" },
        applicationName: "PostgreSQL",
      },
    }
    const second = {
      body: {
        applicationName: "PostgreSQL",
        values: { password: "generated", username: "app" },
      },
      templateId: "postgresql",
      projectId: "prj_1",
    }

    expect(canonicalJSONStringify(first)).toBe(canonicalJSONStringify(second))
    expect(hashCanonicalJSON(first)).toBe(hashCanonicalJSON(second))
  })

  it("keeps arrays ordered and rejects unsupported values", () => {
    expect(canonicalJSONStringify({ values: ["a", "b"] })).not.toBe(canonicalJSONStringify({ values: ["b", "a"] }))
    expect(() => canonicalJSONStringify({ value: Number.NaN })).toThrow("ai.invalid_tool_arguments")
  })
})
