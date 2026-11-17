import { describe, expect, it } from "vitest"
import { parseRetryAfter, retryDelayMs } from "../src/retry.js"

describe("HTTP retry header parsing and exponential backoff", () => {
  it("returns undefined when Retry-After headers are absent instead of coercing null to 0ms", () => {
    expect(parseRetryAfter(new Headers())).toBeUndefined()
    expect(retryDelayMs(1, { maxRetries: 5, initialDelayMs: 500 }, () => 0.5)).toBe(500)
    expect(retryDelayMs(3, { maxRetries: 5, initialDelayMs: 500 }, () => 0.5)).toBe(2_000)
  })

  it("parses retry-after-ms, Retry-After seconds, and HTTP-date values", () => {
    expect(parseRetryAfter(new Headers({ "retry-after-ms": "250" }))).toBe(250)
    expect(parseRetryAfter(new Headers({ "retry-after-ms": "0" }))).toBe(0)
    expect(parseRetryAfter(new Headers({ "retry-after": "3" }))).toBe(3_000)
    const baseNow = Date.parse("2026-11-17T00:00:00.000Z")
    expect(parseRetryAfter(new Headers({ "retry-after": "Tue, 17 Nov 2026 00:00:05 GMT" }), baseNow)).toBe(5_000)
  })
})
