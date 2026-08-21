import { describe, expect, it } from "vitest"
import { deriveInternalKeys } from "./internal-secret.js"

describe("deriveInternalKeys", () => {
  it("matches the shared Go vector", () => {
    const keys = deriveInternalKeys("0123456789abcdef0123456789abcdef")
    expect(keys.serviceToken).toBe("rc9uB_qX7ORPNH5SB-_AhAPh3hgMj0qMkjfVkHqRDco")
    expect(keys.actorSigningKey).toHaveLength(43)
    expect(keys.callbackServiceToken).toHaveLength(43)
    expect(keys.toolArgumentsEncryptionKey).toHaveLength(32)
    expect(keys.callbackServiceToken).not.toBe(keys.serviceToken)
  })

  it("rejects short roots", () => {
    expect(() => deriveInternalKeys("too-short")).toThrow(/at least 32 bytes/)
  })
})
