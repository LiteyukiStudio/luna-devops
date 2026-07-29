import { describe, expect, it } from "vitest"
import { loadConfig } from "../src/config.js"

describe("configuration", () => {
  it("rejects unsafe production defaults", () => {
    expect(() => loadConfig({ NODE_ENV: "production" })).toThrow("DATABASE_URL")
  })
  it("allows deterministic test configuration", () => {
    expect(loadConfig({ NODE_ENV: "test" }).PROVIDER_TYPE).toBe("deterministic")
  })
})
