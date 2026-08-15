type TimelineCursorPayload = {
  version: 1
  conversationId: string
  beforeTurnIndex: number
}

const cursorPattern = /^[A-Za-z0-9_-]+$/
const maximumCursorLength = 512

export function encodeTimelineCursor(conversationId: string, beforeTurnIndex: number): string {
  const payload: TimelineCursorPayload = { version: 1, conversationId, beforeTurnIndex }
  return Buffer.from(JSON.stringify(payload), "utf8").toString("base64url")
}

export function decodeTimelineCursor(cursor: string, conversationId: string): number {
  try {
    if (!cursor || cursor.length > maximumCursorLength || !cursorPattern.test(cursor)) throw new Error("invalid")
    const buffer = Buffer.from(cursor, "base64url")
    if (buffer.toString("base64url") !== cursor) throw new Error("invalid")
    const value: unknown = JSON.parse(buffer.toString("utf8"))
    if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("invalid")
    const payload = value as Partial<TimelineCursorPayload>
    const beforeTurnIndex = payload.beforeTurnIndex
    if (payload.version !== 1
      || payload.conversationId !== conversationId
      || !Number.isSafeInteger(beforeTurnIndex)
      || beforeTurnIndex === undefined
      || beforeTurnIndex < 0)
      throw new Error("invalid")
    return beforeTurnIndex
  }
  catch {
    throw new Error("ai.timeline_cursor_invalid")
  }
}
