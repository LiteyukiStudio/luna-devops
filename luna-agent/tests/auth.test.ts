import { createHmac } from "node:crypto"
import { describe, expect, it } from "vitest"
import { BffHmacAuthenticator } from "../src/auth.js"

describe("BFF Actor Context authentication", () => {
  it("verifies independent service and actor materials", async () => {
    const serviceToken = "s".repeat(32)
    const signingKey = "k".repeat(32)
    const now = Math.floor(Date.now() / 1000)
    const context = Buffer.from(JSON.stringify({
      userId: "usr_a", sessionId: "sess_a", locale: "zh-CN",
      issuedAt: now, expiresAt: now + 60, requestId: "req_a",
    })).toString("base64url")
    const signature = `sha256=${createHmac("sha256", signingKey).update(context).digest("hex")}`
    const actor = await new BffHmacAuthenticator(serviceToken, signingKey).verify({
      authorization: `Bearer ${serviceToken}`,
      "x-luna-actor-context": context,
      "x-luna-actor-signature": signature,
    })
    expect(actor.userId).toBe("usr_a")
  })
  it("rejects a modified Actor Context", async () => {
    const authenticator = new BffHmacAuthenticator("s".repeat(32), "k".repeat(32))
    await expect(authenticator.verify({
      authorization: `Bearer ${"s".repeat(32)}`,
      "x-luna-actor-context": "modified",
      "x-luna-actor-signature": `sha256=${"0".repeat(64)}`,
    })).rejects.toThrow("ai.unauthorized")
  })
})
