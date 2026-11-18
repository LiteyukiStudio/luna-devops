import { createCipheriv, randomBytes } from "node:crypto"
import { describe, expect, it } from "vitest"
import { deriveInternalKeys } from "../src/internal-secret.js"
import { PayloadCipher } from "../src/payload-cipher.js"

describe("PayloadCipher AEAD, row-level AAD, and dual-key rotation", () => {
  it("uses authenticated randomized encryption and restores only through the vault", () => {
    const cipher = new PayloadCipher(Buffer.alloc(32, 7), "test-v1")
    const first = cipher.encrypt("opaque-sensitive-grant")
    const second = cipher.encrypt("opaque-sensitive-grant")
    expect(first).not.toContain("opaque-sensitive-grant")
    expect(first).not.toBe(second)
    expect(cipher.decrypt(first)).toBe("opaque-sensitive-grant")
  })

  it("fails closed with a stable code when the envelope cannot be authenticated or has truncated auth tag", () => {
    const key = Buffer.alloc(32, 7)
    const cipher = new PayloadCipher(key, "test-v1", "ai.payload_unavailable")
    expect(() => cipher.decrypt('{"keyVersion":"test-v1","nonce":"invalid"}')).toThrow("ai.payload_unavailable")

    // Forge an envelope with a 12-byte GCM tag instead of 16 bytes -> must reject strictly
    const nonce = randomBytes(12)
    const shortTagCipher = createCipheriv("aes-256-gcm", key, nonce, { authTagLength: 12 })
    const ciphertext = Buffer.concat([shortTagCipher.update("secret", "utf8"), shortTagCipher.final()])
    const truncatedEnvelope = JSON.stringify({
      keyVersion: "test-v1",
      nonce: nonce.toString("base64url"),
      ciphertext: ciphertext.toString("base64url"),
      tag: shortTagCipher.getAuthTag().toString("base64url"),
    })
    expect(() => cipher.decrypt(truncatedEnvelope)).toThrow("ai.payload_unavailable")
  })

  it("binds row-level AAD to prevent cross-row ciphertext transplantation while reading legacy unbound rows", () => {
    const cipher = new PayloadCipher(Buffer.alloc(32, 11), "tool-arguments-v1", "ai.tool_arguments_key_unavailable")
    const bound = cipher.encrypt('{"token":"secret-1"}', "airun_1:aitool_1")
    expect(JSON.parse(bound)).toMatchObject({ aadBound: true })
    expect(cipher.decrypt(bound, "airun_1:aitool_1")).toBe('{"token":"secret-1"}')

    // Moving ciphertext from airun_1:aitool_1 to airun_1:aitool_2 or omitting AAD must fail
    expect(() => cipher.decrypt(bound, "airun_1:aitool_2")).toThrow("ai.tool_arguments_key_unavailable")
    expect(() => cipher.decrypt(bound)).toThrow("ai.tool_arguments_key_unavailable")

    // Empty-string AAD is rejected on both encrypt and decrypt
    expect(() => cipher.encrypt('{"token":"secret"}', "")).toThrow("AAD")
    expect(() => cipher.decrypt(bound, "")).toThrow("ai.tool_arguments_key_unavailable")

    // Legacy envelope without aadBound remains readable during rolling upgrade
    const legacy = cipher.encrypt('{"legacy":true}')
    expect(JSON.parse(legacy)).not.toHaveProperty("aadBound")
    expect(cipher.decrypt(legacy, "airun_legacy:aitool_legacy")).toBe('{"legacy":true}')
  })

  it("rotates smoothly with AI_INTERNAL_SECRET_PREVIOUS fallback on decrypt while encrypting with primary key", () => {
    const oldKeys = deriveInternalKeys("previous-internal-secret-at-least-32-bytes")
    const newKeys = deriveInternalKeys("current-internal-secret-at-least-32-bytes!")

    const oldCipher = new PayloadCipher(
      oldKeys.toolArgumentsEncryptionKey,
      "tool-arguments-v1",
      "ai.tool_arguments_key_unavailable",
    )
    const rotatedCipher = new PayloadCipher(
      newKeys.toolArgumentsEncryptionKey,
      "tool-arguments-v1",
      "ai.tool_arguments_key_unavailable",
      oldKeys.toolArgumentsEncryptionKey,
    )
    const newOnlyCipher = new PayloadCipher(
      newKeys.toolArgumentsEncryptionKey,
      "tool-arguments-v1",
      "ai.tool_arguments_key_unavailable",
    )

    const oldEnvelope = oldCipher.encrypt('{"apiKey":"old-row"}', "airun_rot:aitool_1")
    expect(rotatedCipher.decrypt(oldEnvelope, "airun_rot:aitool_1")).toBe('{"apiKey":"old-row"}')
    expect(() => rotatedCipher.decrypt(oldEnvelope, "airun_rot:aitool_tampered")).toThrow("ai.tool_arguments_key_unavailable")

    const freshEnvelope = rotatedCipher.encrypt('{"apiKey":"new-row"}', "airun_rot:aitool_2")
    expect(newOnlyCipher.decrypt(freshEnvelope, "airun_rot:aitool_2")).toBe('{"apiKey":"new-row"}')
    expect(() => oldCipher.decrypt(freshEnvelope, "airun_rot:aitool_2")).toThrow("ai.tool_arguments_key_unavailable")
  })
})
