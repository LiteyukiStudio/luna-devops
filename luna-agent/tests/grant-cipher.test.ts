import { describe, expect, it } from "vitest"
import { RunGrantCipher } from "../src/grant-cipher.js"

describe("Run Actor Grant encryption", () => {
  it("uses authenticated randomized encryption and restores only through the vault", () => {
    const cipher = new RunGrantCipher(Buffer.alloc(32, 7), "test-v1")
    const first = cipher.encrypt("opaque-sensitive-grant")
    const second = cipher.encrypt("opaque-sensitive-grant")
    expect(first).not.toContain("opaque-sensitive-grant")
    expect(first).not.toBe(second)
    expect(cipher.decrypt(first)).toBe("opaque-sensitive-grant")
  })
})
