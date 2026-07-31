import { describe, expect, it } from "vitest"
import { deriveInternalKeys } from "./internal-secret.js"

describe("deriveInternalKeys", () => {
  it("matches the shared Go vector", () => {
    const keys = deriveInternalKeys("0123456789abcdef0123456789abcdef")
    expect(keys.serviceToken).toBe("rc9uB_qX7ORPNH5SB-_AhAPh3hgMj0qMkjfVkHqRDco")
    expect(keys.runGrantEncryptionKey.toString("hex")).toBe("68fb9e789fd931374447396477b8964d3ba519f0ca94564026d90262e2f1e7d0")
    expect(keys.toolArgumentsEncryptionKey).toHaveLength(32)
    expect(keys.toolArgumentsEncryptionKey).not.toEqual(keys.runGrantEncryptionKey)
  })

  it("rejects short roots", () => {
    expect(() => deriveInternalKeys("too-short")).toThrow(/at least 32 bytes/)
  })
})
