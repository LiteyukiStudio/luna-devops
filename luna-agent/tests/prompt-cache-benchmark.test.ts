import { describe, expect, it } from "vitest"
import {
  failedPromptCacheBenchmarkAssertions,
  isPromptCacheBenchmarkResult,
  runPromptCacheBenchmark,
  type PromptCacheBenchmarkResult,
  utf8LongestCommonPrefixBytes,
} from "../benchmarks/prompt-cache/benchmark.js"
import {
  comparePromptCacheBenchmarks,
  isPromptCacheBenchmarkComparison,
} from "../benchmarks/prompt-cache/comparison.js"
import {
  promptCacheReportPalettes,
  renderPromptCacheBenchmarkReport,
} from "../benchmarks/prompt-cache/report.js"

process.env.LOG_LEVEL = "error"

const baselineRevision = "1".repeat(40)
const optimizedRevision = "2".repeat(40)

describe("Agent prompt-cache benchmark", () => {
  it("runs every deterministic scenario with functional invariants intact", async () => {
    const first = await runPromptCacheBenchmark({
      checkoutLabel: "baseline",
      sourceRevision: baselineRevision,
    })
    const second = await runPromptCacheBenchmark({
      checkoutLabel: "baseline",
      sourceRevision: baselineRevision,
    })

    expect(second).toEqual(first)
    expect(first.scenarios.map(scenario => scenario.id)).toEqual([
      "same-run-multi-step",
      "cross-turn-history",
      "tool-touch-and-addition",
      "before-and-after-compaction",
    ])
    expect(first.summary).toMatchObject({
      scenarioCount: 4,
      transitionCount: 6,
      assertionCount: 24,
      passedAssertionCount: 24,
      failedAssertionCount: 0,
      cacheEpochInvalidationTransitionCount: 1,
    })
    expect(first.benchmark).toMatchObject({
      sourceRevision: baselineRevision,
    })
    expect(first.benchmark.implementationDigest).toMatch(/^[a-f0-9]{64}$/)
    expect(first.benchmark.harnessDigest).toMatch(/^[a-f0-9]{64}$/)
    expect(failedPromptCacheBenchmarkAssertions(first)).toEqual([])
    for (const step of first.scenarios.flatMap(scenario => scenario.steps))
      expect(step.requestSha256).toMatch(/^[a-f0-9]{64}$/)
    expect(first.scenarios.flatMap(scenario => scenario.transitions).every(transition =>
      transition.commonPrefixBytes > 0
      && transition.estimatedCommonPrefixTokens === Math.ceil(transition.commonPrefixBytes / 4)
      && transition.nextRequestReuseRatio >= 0
      && transition.nextRequestReuseRatio <= 1
      && transition.uncachedSuffixBytes >= 0,
    )).toBe(true)
    expect(first.summary.uncachedSuffixBytes).toBe(first.summary.nextRequestBytes - first.summary.commonPrefixBytes)
    const compaction = first.scenarios.find(scenario => scenario.id === "before-and-after-compaction")!
    expect(compaction.steps.map(step => step.id)).toEqual([
      "before-compaction",
      "after-compaction",
      "summary-reused",
    ])
    expect(compaction.transitions.map(transition => transition.cacheEpochTransition)).toEqual([
      "cache_epoch_invalidation",
      "within_epoch",
    ])
    expect(JSON.stringify(first)).not.toContain("serializedRequest")
    expect(isPromptCacheBenchmarkResult(first)).toBe(true)
    expect(isPromptCacheBenchmarkResult({ ...first, scenarios: [{ id: "incomplete" }] })).toBe(false)
  })

  it("counts UTF-8 bytes instead of JavaScript code units", () => {
    expect(utf8LongestCommonPrefixBytes("缓存-A", "缓存-B")).toBe(Buffer.byteLength("缓存-", "utf8"))
    expect(utf8LongestCommonPrefixBytes("abc", "xyz")).toBe(0)
  })

  it("aligns baseline and optimized results by scenario and transition IDs", async () => {
    const baseline = await benchmarkResult("baseline", baselineRevision)
    const optimized = distinctClone(baseline)
    const expected = improveFirstTransition(optimized, 400)

    const comparison = comparePromptCacheBenchmarks(baseline, optimized)

    expect(comparison.summary).toMatchObject({
      comparableTransitionCount: 6,
      missingBaselineTransitionCount: 0,
      missingOptimizedTransitionCount: 0,
      functionalAssertionsPassed: true,
      commonPrefixBytesDelta: expected.summaryBytesDelta,
      estimatedCommonPrefixTokensDelta: expected.summaryTokenDelta,
      weightedNextRequestReusePercentagePointDelta: expected.summaryPercentagePointDelta,
      uncachedSuffixBytesDelta: expected.summaryUncachedSuffixBytesDelta,
    })
    expect(comparison.transitions[0]?.delta).toMatchObject({
      commonPrefixBytes: expected.transitionBytesDelta,
      estimatedCommonPrefixTokens: expected.transitionTokenDelta,
      nextRequestReusePercentagePoints: expected.transitionPercentagePointDelta,
      uncachedSuffixBytes: expected.transitionUncachedSuffixBytesDelta,
    })
    expect(isPromptCacheBenchmarkComparison(comparison)).toBe(true)
  })

  it("rejects relabelled results from the same implementation", async () => {
    const baseline = await benchmarkResult("baseline", baselineRevision)
    const relabelled = structuredClone(baseline)
    relabelled.benchmark.checkoutLabel = "optimized"
    relabelled.benchmark.sourceRevision = optimizedRevision

    expect(() => comparePromptCacheBenchmarks(baseline, relabelled))
      .toThrow("prompt_cache_benchmark_source_not_distinct")
  })

  it("rejects different benchmark harnesses before comparing implementation results", async () => {
    const baseline = await benchmarkResult("baseline", baselineRevision)
    const optimized = distinctClone(baseline)
    optimized.benchmark.harnessDigest = differentDigest(baseline.benchmark.harnessDigest)

    expect(() => comparePromptCacheBenchmarks(baseline, optimized))
      .toThrow("prompt_cache_benchmark_harness_mismatch")
  })

  it("rejects missing transitions and inconsistent derived values", async () => {
    const valid = await benchmarkResult("baseline", baselineRevision)
    const missingTransition = structuredClone(valid)
    missingTransition.scenarios[0]!.transitions.pop()
    expect(isPromptCacheBenchmarkResult(missingTransition)).toBe(false)

    const malformedRatio = structuredClone(valid)
    malformedRatio.scenarios[0]!.transitions[0]!.nextRequestReuseRatio += 0.000001
    expect(isPromptCacheBenchmarkResult(malformedRatio)).toBe(false)

    const malformedSummary = structuredClone(valid)
    malformedSummary.summary.transitionCount -= 1
    expect(isPromptCacheBenchmarkResult(malformedSummary)).toBe(false)

    const malformedUncachedSuffix = structuredClone(valid)
    malformedUncachedSuffix.scenarios[0]!.transitions[0]!.uncachedSuffixBytes += 1
    expect(isPromptCacheBenchmarkResult(malformedUncachedSuffix)).toBe(false)

    const malformedCacheEpochCount = structuredClone(valid)
    malformedCacheEpochCount.summary.cacheEpochInvalidationTransitionCount += 1
    expect(isPromptCacheBenchmarkResult(malformedCacheEpochCount)).toBe(false)
  })

  it("requires the same scenario, step and assertion contract across checkouts", async () => {
    const baseline = await benchmarkResult("baseline", baselineRevision)
    const optimized = distinctClone(baseline)
    optimized.scenarios[0]!.assertions[0]!.description = "被篡改的断言说明"

    expect(isPromptCacheBenchmarkResult(optimized)).toBe(true)
    expect(() => comparePromptCacheBenchmarks(baseline, optimized))
      .toThrow("prompt_cache_benchmark_assertion_mismatch")
  })
})

describe("Agent prompt-cache HTML report", () => {
  it("renders escaped, self-contained, responsive light/dark and print markup", async () => {
    const result = await runPromptCacheBenchmark({
      checkoutLabel: "optimized </title><script>alert(1)</script>",
      sourceRevision: optimizedRevision,
    })
    const html = renderPromptCacheBenchmarkReport(result)

    expect(html).toContain("<!doctype html>")
    expect(html).toContain("href=\"#main-content\"")
    expect(html).toContain("aria-label=")
    expect(html).toContain("<caption")
    expect(html).toContain("@media (prefers-color-scheme: dark)")
    expect(html).toContain("@media (prefers-reduced-motion: reduce)")
    expect(html).toContain("@media print")
    expect(html).toContain("overflow-x: auto")
    expect(html).toContain("Benchmark 来源校验")
    expect(html).toContain("未复用后缀")
    expect(html).toContain("Cache epoch 失效")
    expect(html).toContain("Cache epoch 分层指标")
    expect(html).toContain(`${result.benchmark.sourceRevision.slice(0, 12)}…`)
    expect(html).toContain(`${result.benchmark.implementationDigest.slice(0, 12)}…`)
    expect(html).toContain(`${result.benchmark.harnessDigest.slice(0, 12)}…`)
    expect(html).toContain("&lt;script&gt;alert(1)&lt;/script&gt;")
    expect(html).not.toMatch(/<script(?:\s|>)/i)
    expect(html).not.toMatch(/https?:\/\//i)
    expect(html).not.toMatch(/\s(?:src|srcset)=/i)
  })

  it("renders a baseline/optimized comparison without relying on color alone", async () => {
    const baseline = await benchmarkResult("baseline", baselineRevision)
    const optimized = distinctClone(baseline)
    const expected = improveFirstTransition(optimized, 400)
    const comparison = comparePromptCacheBenchmarks(baseline, optimized)

    const html = renderPromptCacheBenchmarkReport(comparison)

    expect(html).toContain("baseline → optimized")
    expect(html).toContain(`提升 ${formatSignedDecimal(expected.transitionPercentagePointDelta)} pp`)
    expect(html).toContain(">提升<")
    expect(html).toContain("功能等价断言")
    expect(html).toContain("Baseline 与 optimized 来源校验")
    expect(html).toContain(`${baseline.benchmark.sourceRevision.slice(0, 12)}…`)
    expect(html).toContain(`${optimized.benchmark.sourceRevision.slice(0, 12)}…`)
    expect(html).toContain("共享 Harness")
    expect(html).toContain("Baseline 与 optimized")
    expect(html).toContain("负值更好")
    expect(html).toContain("Baseline 与 optimized 的 cache epoch 分层对比")
    const comparisonRow = html.match(/Baseline 与 optimized[\s\S]*?<tbody>([\s\S]*?)<\/tbody>/)?.[1]
    expect(comparisonRow?.match(/<(?:th|td)\b/g)).toHaveLength(5)
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

async function benchmarkResult(label: string, sourceRevision: string): Promise<PromptCacheBenchmarkResult> {
  return runPromptCacheBenchmark({ checkoutLabel: label, sourceRevision })
}

function distinctClone(baseline: PromptCacheBenchmarkResult): PromptCacheBenchmarkResult {
  const optimized = structuredClone(baseline)
  optimized.benchmark.checkoutLabel = "optimized"
  optimized.benchmark.sourceRevision = optimizedRevision
  optimized.benchmark.implementationDigest = differentDigest(baseline.benchmark.implementationDigest)
  return optimized
}

function differentDigest(value: string): string {
  return value === "f".repeat(64) ? "e".repeat(64) : "f".repeat(64)
}

function improveFirstTransition(result: PromptCacheBenchmarkResult, requestedBytes: number) {
  const scenario = result.scenarios[0]!
  const target = scenario.transitions[0]!
  const nextStep = scenario.steps[1]!
  const before = {
    commonPrefixBytes: target.commonPrefixBytes,
    estimatedCommonPrefixTokens: target.estimatedCommonPrefixTokens,
    nextRequestReuseRatio: target.nextRequestReuseRatio,
    summaryCommonPrefixBytes: result.summary.commonPrefixBytes,
    summaryEstimatedCommonPrefixTokens: result.summary.estimatedCommonPrefixTokens,
    summaryWeightedNextRequestReuseRatio: result.summary.weightedNextRequestReuseRatio,
    summaryUncachedSuffixBytes: result.summary.uncachedSuffixBytes,
  }
  const transitionBytesDelta = Math.min(requestedBytes, nextStep.requestBytes - target.commonPrefixBytes)
  if (transitionBytesDelta <= 0) throw new Error("prompt_cache_benchmark_test_transition_cannot_improve")
  target.commonPrefixBytes += transitionBytesDelta
  target.estimatedCommonPrefixTokens = Math.ceil(target.commonPrefixBytes / 4)
  target.nextRequestReuseRatio = roundedRatio(target.commonPrefixBytes, nextStep.requestBytes)
  target.uncachedSuffixBytes = nextStep.requestBytes - target.commonPrefixBytes
  refreshSummary(result)
  return {
    transitionBytesDelta,
    transitionTokenDelta: target.estimatedCommonPrefixTokens - before.estimatedCommonPrefixTokens,
    transitionPercentagePointDelta: percentagePointDelta(
      target.nextRequestReuseRatio,
      before.nextRequestReuseRatio,
    ),
    transitionUncachedSuffixBytesDelta: -transitionBytesDelta,
    summaryBytesDelta: result.summary.commonPrefixBytes - before.summaryCommonPrefixBytes,
    summaryTokenDelta: result.summary.estimatedCommonPrefixTokens - before.summaryEstimatedCommonPrefixTokens,
    summaryPercentagePointDelta: percentagePointDelta(
      result.summary.weightedNextRequestReuseRatio,
      before.summaryWeightedNextRequestReuseRatio,
    ),
    summaryUncachedSuffixBytesDelta: result.summary.uncachedSuffixBytes - before.summaryUncachedSuffixBytes,
  }
}

function refreshSummary(result: PromptCacheBenchmarkResult): void {
  const transitions = result.scenarios.flatMap(scenario => scenario.transitions)
  const assertions = result.scenarios.flatMap(scenario => scenario.assertions)
  const commonPrefixBytes = transitions.reduce((total, transition) => total + transition.commonPrefixBytes, 0)
  const nextRequestBytes = result.scenarios.reduce((total, scenario) => total
    + scenario.steps.slice(1).reduce((scenarioTotal, step) => scenarioTotal + step.requestBytes, 0), 0)
  const uncachedSuffixBytes = transitions.reduce(
    (total, transition) => total + transition.uncachedSuffixBytes,
    0,
  )
  result.summary = {
    scenarioCount: result.scenarios.length,
    transitionCount: transitions.length,
    assertionCount: assertions.length,
    passedAssertionCount: assertions.filter(assertion => assertion.passed).length,
    failedAssertionCount: assertions.filter(assertion => !assertion.passed).length,
    commonPrefixBytes,
    estimatedCommonPrefixTokens: Math.ceil(commonPrefixBytes / 4),
    nextRequestBytes,
    uncachedSuffixBytes,
    cacheEpochInvalidationTransitionCount: transitions.filter(
      transition => transition.cacheEpochTransition === "cache_epoch_invalidation",
    ).length,
    weightedNextRequestReuseRatio: roundedRatio(commonPrefixBytes, nextRequestBytes),
  }
}

function roundedRatio(numerator: number, denominator: number): number {
  return denominator > 0 ? Number((numerator / denominator).toFixed(6)) : 0
}

function percentagePointDelta(optimized: number, baseline: number): number {
  return Number(((optimized - baseline) * 100).toFixed(4))
}

function formatSignedDecimal(value: number): string {
  return `${value > 0 ? "+" : value < 0 ? "-" : ""}${Math.abs(value).toFixed(2)}`
}

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
