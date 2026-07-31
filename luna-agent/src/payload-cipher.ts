import { createCipheriv, createDecipheriv, randomBytes } from "node:crypto"

export class PayloadCipher {
  constructor(
    private readonly key: Buffer,
    private readonly keyVersion = "v1",
    private readonly unavailableCode = "ai.encryption_key_unavailable",
  ) {
    if (key.length !== 32) throw new Error("Payload encryption key must be 32 bytes")
  }
  encrypt(value: string): string {
    const nonce = randomBytes(12)
    const cipher = createCipheriv("aes-256-gcm", this.key, nonce)
    const encrypted = Buffer.concat([cipher.update(value, "utf8"), cipher.final()])
    return JSON.stringify({ keyVersion: this.keyVersion, nonce: nonce.toString("base64url"), ciphertext: encrypted.toString("base64url"), tag: cipher.getAuthTag().toString("base64url") })
  }
  decrypt(envelope: string): string {
    try {
      const value = JSON.parse(envelope) as { keyVersion: string, nonce: string, ciphertext: string, tag: string }
      if (value.keyVersion !== this.keyVersion)
        throw new Error(this.unavailableCode)
      const decipher = createDecipheriv("aes-256-gcm", this.key, Buffer.from(value.nonce, "base64url"))
      decipher.setAuthTag(Buffer.from(value.tag, "base64url"))
      return Buffer.concat([decipher.update(Buffer.from(value.ciphertext, "base64url")), decipher.final()]).toString("utf8")
    }
    catch {
      throw new Error(this.unavailableCode)
    }
  }
}
