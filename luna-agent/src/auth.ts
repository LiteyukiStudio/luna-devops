import { createHmac, timingSafeEqual } from "node:crypto"
import { z } from "zod"
import type { ActorContext } from "./domain.js"

const actorSchema = z.object({
  userId: z.string().min(1),
  sessionId: z.string().min(1),
  projectId: z.string().optional(),
  locale: z.string().default("zh-CN"),
  issuedAt: z.number().int(),
  expiresAt: z.number().int(),
  requestId: z.string().min(1),
  runId: z.string().optional(),
})

export interface RequestVerifier {
  verify(headers: Record<string, string | string[] | undefined>): Promise<ActorContext>
}

export class DevelopmentRequestVerifier implements RequestVerifier {
  async verify(headers: Record<string, string | string[] | undefined>): Promise<ActorContext> {
    const userId = scalar(headers["x-luna-dev-user"])
    const sessionId = scalar(headers["x-luna-dev-session"]) ?? "dev-session"
    if (!userId) throw new AuthError("ai.unauthorized")
    const now = Math.floor(Date.now() / 1000)
    return { userId, sessionId, locale: scalar(headers["x-luna-locale"]) ?? "zh-CN", issuedAt: now, expiresAt: now + 60, requestId: scalar(headers["x-request-id"]) ?? crypto.randomUUID() }
  }
}

export class BffHmacRequestVerifier implements RequestVerifier {
  constructor(private readonly serviceToken: string, private readonly actorSigningKey: string) {}
  async verify(headers: Record<string, string | string[] | undefined>): Promise<ActorContext> {
    const bearer = scalar(headers.authorization)?.replace(/^Bearer\s+/i, "")
    const encoded = scalar(headers["x-luna-actor-context"])
    const signature = scalar(headers["x-luna-actor-signature"])
    if (!bearer || !encoded || !signature || !safeEqual(bearer, this.serviceToken)) throw new AuthError("ai.unauthorized")
    const expected = `sha256=${createHmac("sha256", this.actorSigningKey).update(encoded).digest("hex")}`
    if (!safeEqual(signature, expected)) throw new AuthError("ai.unauthorized")
    let raw: unknown
    try { raw = JSON.parse(Buffer.from(encoded, "base64url").toString("utf8")) } catch { throw new AuthError("ai.actor_context_invalid") }
    const actor = actorSchema.parse(raw)
    const now = Math.floor(Date.now() / 1000)
    if (actor.issuedAt > now + 5 || actor.expiresAt < now || actor.expiresAt - actor.issuedAt > 60) throw new AuthError("ai.actor_context_expired")
    return {
      userId: actor.userId, sessionId: actor.sessionId, locale: actor.locale,
      issuedAt: actor.issuedAt, expiresAt: actor.expiresAt, requestId: actor.requestId,
      ...(actor.projectId ? { projectId: actor.projectId } : {}),
      ...(actor.runId ? { runId: actor.runId } : {}),
    }
  }
}

export class AuthError extends Error {}
function scalar(value: string | string[] | undefined) { return Array.isArray(value) ? value[0] : value }
function safeEqual(left: string, right: string): boolean {
  const a = Buffer.from(left)
  const b = Buffer.from(right)
  return a.length === b.length && timingSafeEqual(a, b)
}
