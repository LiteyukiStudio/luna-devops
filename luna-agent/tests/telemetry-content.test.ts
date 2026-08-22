import { describe, expect, it, vi } from "vitest"
import type { Span } from "@opentelemetry/api"
import { configureAIContentCapture, recordAIContent, serializeAIContent } from "../src/telemetry.js"

describe("AI content telemetry", () => {
  it("redacts secrets before serializing content", () => {
    const content = serializeAIContent({
      messages: [{ content: "Authorization: Bearer raw-token" }],
      arguments: { password: "database-secret" },
    })

    expect(content.truncated).toBe(false)
    expect(content.value).toContain("[REDACTED]")
    expect(content.value).not.toContain("raw-token")
    expect(content.value).not.toContain("database-secret")
  })

  it("caps exported content fields", () => {
    const content = serializeAIContent({ content: "a".repeat(128) }, 32)
    expect(content.truncated).toBe(true)
    expect(content.value.length).toBe(32)
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
      expect(attributes["luna.ai.content.truncated"]).toBe(false)
      expect(JSON.stringify(write.mock.calls)).not.toContain("visible")
    }
    finally {
      configureAIContentCapture(false)
      write.mockRestore()
    }
  })

  it("omits overlong content instead of emitting invalid JSON", () => {
    const attributes: Record<string, unknown> = {}
    const span = {
      setAttribute(name: string, value: unknown) {
        attributes[name] = value
        return this
      },
    } as unknown as Span

    configureAIContentCapture(true)
    recordAIContent(span, "luna.gen_ai.content.input", "gen_ai.input.messages", { content: "a".repeat(40_000) })
    expect(attributes["gen_ai.input.messages"]).toBeUndefined()
    expect(attributes["luna.ai.content.truncated"]).toBe(true)
    configureAIContentCapture(false)
  })
})
