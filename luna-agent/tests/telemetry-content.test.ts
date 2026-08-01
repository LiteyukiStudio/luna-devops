import { describe, expect, it } from "vitest"
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

  it("does not add content events unless capture is explicitly enabled", () => {
    const events: Array<{ name: string, attributes?: unknown }> = []
    const span = {
      addEvent(name: string, attributes?: unknown) {
        events.push({ name, attributes })
        return this
      },
    } as unknown as Span

    configureAIContentCapture(false)
    recordAIContent(span, "gen_ai.content.input", "gen_ai.input.messages", { content: "hidden" })
    expect(events).toHaveLength(0)

    configureAIContentCapture(true)
    recordAIContent(span, "gen_ai.content.input", "gen_ai.input.messages", { content: "visible" })
    expect(events).toHaveLength(1)
    expect(events[0]?.name).toBe("gen_ai.content.input")
    configureAIContentCapture(false)
  })
})
