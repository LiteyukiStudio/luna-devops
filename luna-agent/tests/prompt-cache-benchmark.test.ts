import { describe, expect, it } from "vitest"
import {
  failedPromptCacheBenchmarkAssertions,
  isPromptCacheBenchmarkResult,
  runPromptCacheBenchmark,
  utf8LongestCommonPrefixBytes,
} from "../benchmarks/prompt-cache/benchmark.js"
import { comparePromptCacheBenchmarks } from "../benchmarks/prompt-cache/comparison.js"
import {
  promptCacheReportPalettes,
  renderPromptCacheBenchmarkReport,
} from "../benchmarks/prompt-cache/report.js"

process.env.LOG_LEVEL = "error"

describe("Agent prompt-cache benchmark", () => {
  it("runs every deterministic scenario with functional invariants intact", async () => {
    const first = await runPromptCacheBenchmark({ checkoutLabel: "baseline" })
    const second = await runPromptCacheBenchmark({ checkoutLabel: "baseline" })

    expect(second).toEqual(first)
    expect(first.scenarios.map(scenario => scenario.id)).toEqual([
      "same-run-multi-step",
      "cross-turn-history",
      "tool-touch-and-addition",
      "before-and-after-compaction",
    ])
    expect(first.summary).toMatchObject({
      scenarioCount: 4,
      transitionCount: 5,
      assertionCount: 18,
      passedAssertionCount: 18,
      failedAssertionCount: 0,
    })
    expect(failedPromptCacheBenchmarkAssertions(first)).toEqual([])
    for (const step of first.scenarios.flatMap(scenario => scenario.steps))
      expect(step.requestSha256).toMatch(/^[a-f0-9]{64}$/)
    expect(first.scenarios.flatMap(scenario => scenario.transitions).every(transition =>
      transition.commonPrefixBytes > 0
      && transition.estimatedCommonPrefixTokens === Math.ceil(transition.commonPrefixBytes / 4)
      && transition.nextRequestReuseRatio >= 0
      && transition.nextRequestReuseRatio <= 1,
    )).toBe(true)
    expect(JSON.stringify(first)).not.toContain("serializedRequest")
    expect(isPromptCacheBenchmarkResult(first)).toBe(true)
    expect(isPromptCacheBenchmarkResult({ ...first, scenarios: [{ id: "incomplete" }] })).toBe(false)
  })

  it("counts UTF-8 bytes instead of JavaScript code units", () => {
    expect(utf8LongestCommonPrefixBytes("缓存-A", "缓存-B")).toBe(Buffer.byteLength("缓存-", "utf8"))
    expect(utf8LongestCommonPrefixBytes("abc", "xyz")).toBe(0)
  })

  it("aligns baseline and optimized results by scenario and transition IDs", async () => {
    const baseline = await runPromptCacheBenchmark({ checkoutLabel: "baseline" })
    const optimized = structuredClone(baseline)
    optimized.benchmark.checkoutLabel = "optimized"
    const target = optimized.scenarios[0]!.transitions[0]!
    target.commonPrefixBytes += 400
    target.estimatedCommonPrefixTokens += 100
    target.nextRequestReuseRatio = Number((target.nextRequestReuseRatio + 0.01).toFixed(6))
    optimized.summary.commonPrefixBytes += 400
    optimized.summary.estimatedCommonPrefixTokens += 100
    optimized.summary.weightedNextRequestReuseRatio = Number((
      optimized.summary.weightedNextRequestReuseRatio + 0.01
    ).toFixed(6))

    const comparison = comparePromptCacheBenchmarks(baseline, optimized)

    expect(comparison.summary).toMatchObject({
      comparableTransitionCount: 5,
      missingBaselineTransitionCount: 0,
      missingOptimizedTransitionCount: 0,
      functionalAssertionsPassed: true,
      commonPrefixBytesDelta: 400,
      estimatedCommonPrefixTokensDelta: 100,
      weightedNextRequestReusePercentagePointDelta: 1,
    })
    expect(comparison.transitions[0]?.delta).toMatchObject({
      commonPrefixBytes: 400,
      estimatedCommonPrefixTokens: 100,
      nextRequestReusePercentagePoints: 1,
    })
  })
})

describe("Agent prompt-cache HTML report", () => {
  it("renders escaped, self-contained, responsive light/dark and print markup", async () => {
    const result = await runPromptCacheBenchmark({ checkoutLabel: "optimized </title><script>alert(1)</script>" })
    const html = renderPromptCacheBenchmarkReport(result)

    expect(html).toContain("<!doctype html>")
    expect(html).toContain("href=\"#main-content\"")
    expect(html).toContain("aria-label=")
    expect(html).toContain("<caption")
    expect(html).toContain("@media (prefers-color-scheme: dark)")
    expect(html).toContain("@media (prefers-reduced-motion: reduce)")
    expect(html).toContain("@media print")
    expect(html).toContain("overflow-x: auto")
    expect(html).toContain("&lt;script&gt;alert(1)&lt;/script&gt;")
    expect(html).not.toMatch(/<script(?:\s|>)/i)
    expect(html).not.toMatch(/https?:\/\//i)
    expect(html).not.toMatch(/\s(?:src|srcset)=/i)
  })

  it("renders a baseline/optimized comparison without relying on color alone", async () => {
    const baseline = await runPromptCacheBenchmark({ checkoutLabel: "baseline" })
    const optimized = structuredClone(baseline)
    optimized.benchmark.checkoutLabel = "optimized"
    optimized.summary.weightedNextRequestReuseRatio += 0.01
    const comparison = comparePromptCacheBenchmarks(baseline, optimized)

    const html = renderPromptCacheBenchmarkReport(comparison)

    expect(html).toContain("baseline → optimized")
    expect(html).toContain(">+1.00 pp<")
    expect(html).toContain(">提升<")
    expect(html).toContain("功能等价断言")
    expect(html).toContain("Baseline 与 optimized")
    const comparisonRow = html.match(/Baseline 与 optimized[\s\S]*?<tbody>([\s\S]*?)<\/tbody>/)?.[1]
    expect(comparisonRow?.match(/<(?:th|td)\b/g)).toHaveLength(4)
  })

  it("keeps every semantic text color above WCAG AA contrast on report surfaces", () => {
    for (const palette of Object.values(promptCacheReportPalettes)) {
      for (const background of [palette.background, palette.surface]) {
        for (const foreground of [
          palette.text,
          palette.muted,
          palette.accent,
          palette.success,
          palette.warning,
          palette.danger,
        ]) {
          expect(contrastRatio(foreground, background)).toBeGreaterThanOrEqual(4.5)
        }
      }
    }
  })
})

function contrastRatio(left: string, right: string): number {
  const leftLuminance = relativeLuminance(left)
  const rightLuminance = relativeLuminance(right)
  return (Math.max(leftLuminance, rightLuminance) + 0.05)
    / (Math.min(leftLuminance, rightLuminance) + 0.05)
}

function relativeLuminance(hex: string): number {
  const channels = hex.slice(1).match(/.{2}/g)?.map(value => Number.parseInt(value, 16) / 255)
  if (!channels || channels.length !== 3) throw new Error(`invalid_hex_color:${hex}`)
  const linear = channels.map(value => value <= 0.04045
    ? value / 12.92
    : ((value + 0.055) / 1.055) ** 2.4)
  return 0.2126 * linear[0]! + 0.7152 * linear[1]! + 0.0722 * linear[2]!
}
