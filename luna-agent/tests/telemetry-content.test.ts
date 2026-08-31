import { describe, expect, it, vi } from "vitest"
import type { Span } from "@opentelemetry/api"
import { configureAIContentCapture, recordAIContent, serializeAIContent } from "../src/telemetry.js"

describe("AI content telemetry", () => {
  it("preserves explicitly captured model content", () => {
    const content = serializeAIContent({
      messages: [{ content: "Authorization: Bearer raw-token" }],
      arguments: { password: "database-secret" },
    })

    expect(content.truncated).toBe(false)
    expect(JSON.parse(content.value)).toEqual({
      messages: [{ content: "Authorization: Bearer raw-token" }],
      arguments: { password: "database-secret" },
    })
  })

  it("uses UTF-8 bytes for the content limit", () => {
    const value = { content: "你" }
    const serializedBytes = Buffer.byteLength(JSON.stringify(value), "utf8")

    expect(serializeAIContent(value, serializedBytes)).toMatchObject({ truncated: false, byteLength: serializedBytes })
    expect(serializeAIContent(value, serializedBytes - 1)).toMatchObject({ truncated: true, byteLength: serializedBytes })
  })

  it("never breaks the request when content contains bigint", () => {
    expect(serializeAIContent({ count: 12n }).value).toBe('{"count":"12"}')
  })

  it("keeps captured content on the controlled span and out of Pino logs", () => {
    const attributes: Record<string, unknown> = {}
    const span = {
      setAttribute(name: string, value: unknown) {
        attributes[name] = value
        return this
      },
    } as unknown as Span

    const write = vi.spyOn(process.stderr, "write").mockImplementation(() => true)
    try {
      configureAIContentCapture(false)
      recordAIContent(span, "luna.gen_ai.content.input", "gen_ai.input.messages", { content: "hidden" })
      expect(attributes).toEqual({})

      configureAIContentCapture(true)
      recordAIContent(span, "luna.gen_ai.content.input", "gen_ai.input.messages", { content: "visible" })
      expect(JSON.parse(attributes["gen_ai.input.messages"] as string)).toEqual({ content: "visible" })
      expect(attributes["luna.ai.content.truncated"]).toBeUndefined()
      expect(JSON.stringify(write.mock.calls)).not.toContain("visible")
    }
    finally {
      configureAIContentCapture(false)
      write.mockRestore()
    }
  })

  it("omits overlong content instead of emitting invalid JSON", () => {
    const attributes: Record<string, unknown> = {}
    const events: Array<{ name: string, attributes: Record<string, unknown> | undefined }> = []
    const span = {
      setAttribute(name: string, value: unknown) {
        attributes[name] = value
        return this
      },
      addEvent(name: string, eventAttributes?: Record<string, unknown>) {
        events.push({ name, attributes: eventAttributes })
        return this
      },
    } as unknown as Span

    try {
      configureAIContentCapture(true)
      recordAIContent(span, "luna.gen_ai.content.input", "gen_ai.input.messages", { content: "a".repeat(128 * 1024) })
      recordAIContent(span, "luna.gen_ai.content.output", "gen_ai.output.messages", { content: "short" })

      expect(attributes["gen_ai.input.messages"]).toBeUndefined()
      expect(JSON.parse(attributes["gen_ai.output.messages"] as string)).toEqual({ content: "short" })
      expect(attributes["luna.ai.content.truncated"]).toBe(true)
      expect(events).toEqual([{
        name: "luna.ai.content.omitted",
        attributes: {
          "luna.ai.content.field": "gen_ai.input.messages",
          "luna.ai.content.size_bytes": 131_086,
          "luna.ai.content.limit_bytes": 131_072,
        },
      }])
    }
    finally {
      configureAIContentCapture(false)
    }
  })
})
