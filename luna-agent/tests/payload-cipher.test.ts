import { describe, expect, it } from "vitest"
import { PayloadCipher } from "../src/payload-cipher.js"

describe("Run Actor Grant encryption", () => {
  it("uses authenticated randomized encryption and restores only through the vault", () => {
    const cipher = new PayloadCipher(Buffer.alloc(32, 7), "test-v1")
    const first = cipher.encrypt("opaque-sensitive-grant")
    const second = cipher.encrypt("opaque-sensitive-grant")
    expect(first).not.toContain("opaque-sensitive-grant")
    expect(first).not.toBe(second)
    expect(cipher.decrypt(first)).toBe("opaque-sensitive-grant")
  })
  it("fails closed with a stable code when the envelope cannot be authenticated", () => {
    const cipher = new PayloadCipher(Buffer.alloc(32, 7), "test-v1", "ai.payload_unavailable")
    expect(() => cipher.decrypt('{"keyVersion":"test-v1","nonce":"invalid"}')).toThrow("ai.payload_unavailable")
  })
})
