import { createCipheriv, createDecipheriv, randomBytes } from "node:crypto"

type PayloadEnvelope = {
  keyVersion?: unknown
  aadBound?: unknown
  nonce?: unknown
  ciphertext?: unknown
  tag?: unknown
}

export class PayloadCipher {
  constructor(
    private readonly key: Buffer,
    private readonly keyVersion = "v1",
    private readonly unavailableCode = "ai.encryption_key_unavailable",
    private readonly previousKey?: Buffer,
  ) {
    if (key.length !== 32) throw new Error("Payload encryption key must be 32 bytes")
    if (previousKey !== undefined && previousKey.length !== 32)
      throw new Error("Previous payload encryption key must be 32 bytes")
  }

  encrypt(value: string, aad?: string): string {
    if (aad !== undefined && aad.length === 0) {
      throw new Error("Payload encryption AAD must not be empty")
    }
    const nonce = randomBytes(12)
    const cipher = createCipheriv("aes-256-gcm", this.key, nonce, { authTagLength: 16 })
    const aadBound = aad !== undefined
    if (aadBound) {
      cipher.setAAD(Buffer.from(aad, "utf8"))
    }
    const encrypted = Buffer.concat([cipher.update(value, "utf8"), cipher.final()])
    const tag = cipher.getAuthTag()
    return JSON.stringify({
      keyVersion: this.keyVersion,
      ...(aadBound ? { aadBound: true } : {}),
      nonce: nonce.toString("base64url"),
      ciphertext: encrypted.toString("base64url"),
      tag: tag.toString("base64url"),
    })
  }

  decrypt(envelope: string, aad?: string): string {
    try {
      if (aad !== undefined && aad.length === 0) {
        throw new Error(this.unavailableCode)
      }
      const value = JSON.parse(envelope) as PayloadEnvelope
      if (
        value.keyVersion !== this.keyVersion
        || typeof value.nonce !== "string"
        || typeof value.ciphertext !== "string"
        || typeof value.tag !== "string"
        || (value.aadBound !== undefined && typeof value.aadBound !== "boolean")
      ) {
        throw new Error(this.unavailableCode)
      }
      const nonce = Buffer.from(value.nonce, "base64url")
      const tag = Buffer.from(value.tag, "base64url")
      const ciphertext = Buffer.from(value.ciphertext, "base64url")
      if (nonce.length !== 12 || tag.length !== 16) {
        throw new Error(this.unavailableCode)
      }
      const aadBuffer = value.aadBound === true
        ? (() => {
            if (aad === undefined) throw new Error(this.unavailableCode)
            return Buffer.from(aad, "utf8")
          })()
        : undefined
      const decryptWith = (candidateKey: Buffer): string => {
        const decipher = createDecipheriv("aes-256-gcm", candidateKey, nonce, { authTagLength: 16 })
        if (aadBuffer !== undefined) {
          decipher.setAAD(aadBuffer)
        }
        decipher.setAuthTag(tag)
        return Buffer.concat([decipher.update(ciphertext), decipher.final()]).toString("utf8")
      }
      try {
        return decryptWith(this.key)
      }
      catch (primaryError) {
        if (!this.previousKey) throw primaryError
        return decryptWith(this.previousKey)
      }
    }
    catch {
      throw new Error(this.unavailableCode)
    }
  }
}
