import {
  isPromptCacheBenchmarkResult,
  type PromptCacheBenchmarkResult,
  type PromptCacheBenchmarkScenario,
  type PromptCacheBenchmarkTransition,
} from "./benchmark.js"

export const promptCacheBenchmarkComparisonSchemaVersion = "luna.agent.prompt-cache-benchmark-comparison.v3" as const

export type PromptCacheBenchmarkComparisonTransition = {
  scenarioId: string
  scenarioTitle: string
  fromStepId: string
  toStepId: string
  cacheEpochTransition: PromptCacheBenchmarkTransition["cacheEpochTransition"]
  baseline: TransitionSnapshot
  optimized: TransitionSnapshot
  delta: {
    commonPrefixBytes: number
    estimatedCommonPrefixTokens: number
    nextRequestReusePercentagePoints: number
    uncachedSuffixBytes: number
  }
}

export type PromptCacheBenchmarkComparison = {
  schemaVersion: typeof promptCacheBenchmarkComparisonSchemaVersion
  baseline: PromptCacheBenchmarkResult
  optimized: PromptCacheBenchmarkResult
  summary: {
    comparableTransitionCount: number
    missingBaselineTransitionCount: 0
    missingOptimizedTransitionCount: 0
    functionalAssertionsPassed: boolean
    weightedNextRequestReusePercentagePointDelta: number
    commonPrefixBytesDelta: number
    estimatedCommonPrefixTokensDelta: number
    uncachedSuffixBytesDelta: number
  }
  transitions: PromptCacheBenchmarkComparisonTransition[]
}

type TransitionSnapshot = {
  commonPrefixBytes: number
  estimatedCommonPrefixTokens: number
  nextRequestReuseRatio: number
  nextRequestBytes: number
  uncachedSuffixBytes: number
}

export function comparePromptCacheBenchmarks(
  baseline: PromptCacheBenchmarkResult,
  optimized: PromptCacheBenchmarkResult,
): PromptCacheBenchmarkComparison {
  if (!isPromptCacheBenchmarkResult(baseline))
    throw new Error("prompt_cache_benchmark_baseline_invalid")
  if (!isPromptCacheBenchmarkResult(optimized))
    throw new Error("prompt_cache_benchmark_optimized_invalid")
  if (baseline.benchmark.harnessDigest !== optimized.benchmark.harnessDigest)
    throw new Error("prompt_cache_benchmark_harness_mismatch")
  if (baseline.benchmark.implementationDigest === optimized.benchmark.implementationDigest)
    throw new Error("prompt_cache_benchmark_source_not_distinct")
  assertComparableStructure(baseline, optimized)

  const transitions = baseline.scenarios.flatMap((baselineScenario, scenarioIndex) => {
    const optimizedScenario = optimized.scenarios[scenarioIndex]!
    return baselineScenario.transitions.map((baselineTransition, transitionIndex) => {
      const optimizedTransition = optimizedScenario.transitions[transitionIndex]!
      const baselineSnapshot = snapshot(baselineScenario, baselineTransition)
      const optimizedSnapshot = snapshot(optimizedScenario, optimizedTransition)
      return {
        scenarioId: baselineScenario.id,
        scenarioTitle: baselineScenario.title,
        fromStepId: baselineTransition.fromStepId,
        toStepId: baselineTransition.toStepId,
        cacheEpochTransition: baselineTransition.cacheEpochTransition,
        baseline: baselineSnapshot,
        optimized: optimizedSnapshot,
        delta: {
          commonPrefixBytes: optimizedSnapshot.commonPrefixBytes - baselineSnapshot.commonPrefixBytes,
          estimatedCommonPrefixTokens: optimizedSnapshot.estimatedCommonPrefixTokens
            - baselineSnapshot.estimatedCommonPrefixTokens,
          nextRequestReusePercentagePoints: percentagePointDifference(
            optimizedSnapshot.nextRequestReuseRatio,
            baselineSnapshot.nextRequestReuseRatio,
          ),
          uncachedSuffixBytes: optimizedSnapshot.uncachedSuffixBytes - baselineSnapshot.uncachedSuffixBytes,
        },
      }
    })
  })

  return {
    schemaVersion: promptCacheBenchmarkComparisonSchemaVersion,
    baseline,
    optimized,
    summary: {
      comparableTransitionCount: transitions.length,
      missingBaselineTransitionCount: 0,
      missingOptimizedTransitionCount: 0,
      functionalAssertionsPassed: baseline.summary.failedAssertionCount === 0
        && optimized.summary.failedAssertionCount === 0,
      weightedNextRequestReusePercentagePointDelta: percentagePointDifference(
        optimized.summary.weightedNextRequestReuseRatio,
        baseline.summary.weightedNextRequestReuseRatio,
      ),
      commonPrefixBytesDelta: optimized.summary.commonPrefixBytes - baseline.summary.commonPrefixBytes,
      estimatedCommonPrefixTokensDelta: optimized.summary.estimatedCommonPrefixTokens
        - baseline.summary.estimatedCommonPrefixTokens,
      uncachedSuffixBytesDelta: optimized.summary.uncachedSuffixBytes - baseline.summary.uncachedSuffixBytes,
    },
    transitions,
  }
}

export function isPromptCacheBenchmarkComparison(value: unknown): value is PromptCacheBenchmarkComparison {
  if (!isRecord(value)
    || !hasExactKeys(value, ["schemaVersion", "baseline", "optimized", "summary", "transitions"])
    || value.schemaVersion !== promptCacheBenchmarkComparisonSchemaVersion
    || !isPromptCacheBenchmarkResult(value.baseline)
    || !isPromptCacheBenchmarkResult(value.optimized)) return false

  let expected: PromptCacheBenchmarkComparison
  try {
    expected = comparePromptCacheBenchmarks(value.baseline, value.optimized)
  }
  catch {
    return false
  }
  if (!isRecord(value.summary)
    || !hasExactKeys(value.summary, [
      "comparableTransitionCount",
      "missingBaselineTransitionCount",
      "missingOptimizedTransitionCount",
      "functionalAssertionsPassed",
      "weightedNextRequestReusePercentagePointDelta",
      "commonPrefixBytesDelta",
      "estimatedCommonPrefixTokensDelta",
      "uncachedSuffixBytesDelta",
    ])
    || !sameComparisonSummary(value.summary, expected.summary)
    || !Array.isArray(value.transitions)
    || value.transitions.length !== expected.transitions.length) return false

  return value.transitions.every((transition, index) =>
    sameComparisonTransition(transition, expected.transitions[index]!))
}

function assertComparableStructure(
  baseline: PromptCacheBenchmarkResult,
  optimized: PromptCacheBenchmarkResult,
): void {
  if (baseline.scenarios.length !== optimized.scenarios.length)
    throw new Error("prompt_cache_benchmark_scenario_mismatch")
  baseline.scenarios.forEach((baselineScenario, scenarioIndex) => {
    const optimizedScenario = optimized.scenarios[scenarioIndex]
    if (!optimizedScenario
      || baselineScenario.id !== optimizedScenario.id
      || baselineScenario.title !== optimizedScenario.title
      || baselineScenario.description !== optimizedScenario.description)
      throw new Error(`prompt_cache_benchmark_scenario_mismatch:${baselineScenario.id}`)
    assertSameSequence(
      baselineScenario.steps,
      optimizedScenario.steps,
      step => `${step.id}\0${step.label}`,
      `prompt_cache_benchmark_step_mismatch:${baselineScenario.id}`,
    )
    assertSameSequence(
      baselineScenario.transitions,
      optimizedScenario.transitions,
      transition => `${transition.fromStepId}\0${transition.toStepId}\0${transition.cacheEpochTransition}`,
      `prompt_cache_benchmark_transition_mismatch:${baselineScenario.id}`,
    )
    assertSameSequence(
      baselineScenario.assertions,
      optimizedScenario.assertions,
      assertion => `${assertion.id}\0${assertion.description}`,
      `prompt_cache_benchmark_assertion_mismatch:${baselineScenario.id}`,
    )
  })
}

function assertSameSequence<T>(
  baseline: T[],
  optimized: T[],
  identity: (value: T) => string,
  errorCode: string,
): void {
  if (baseline.length !== optimized.length
    || baseline.some((value, index) => identity(value) !== identity(optimized[index]!)))
    throw new Error(errorCode)
}

function snapshot(
  scenario: PromptCacheBenchmarkScenario,
  transition: PromptCacheBenchmarkTransition,
): TransitionSnapshot {
  const nextRequestBytes = scenario.steps.find(step => step.id === transition.toStepId)?.requestBytes
  if (nextRequestBytes === undefined)
    throw new Error(`prompt_cache_benchmark_transition_target_missing:${scenario.id}:${transition.toStepId}`)
  return {
    commonPrefixBytes: transition.commonPrefixBytes,
    estimatedCommonPrefixTokens: transition.estimatedCommonPrefixTokens,
    nextRequestReuseRatio: transition.nextRequestReuseRatio,
    nextRequestBytes,
    uncachedSuffixBytes: transition.uncachedSuffixBytes,
  }
}

function percentagePointDifference(optimized: number, baseline: number): number {
  return Number(((optimized - baseline) * 100).toFixed(4))
}

function sameComparisonSummary(
  candidate: Record<string, unknown>,
  expected: PromptCacheBenchmarkComparison["summary"],
): boolean {
  return candidate.comparableTransitionCount === expected.comparableTransitionCount
    && candidate.missingBaselineTransitionCount === expected.missingBaselineTransitionCount
    && candidate.missingOptimizedTransitionCount === expected.missingOptimizedTransitionCount
    && candidate.functionalAssertionsPassed === expected.functionalAssertionsPassed
    && candidate.weightedNextRequestReusePercentagePointDelta
      === expected.weightedNextRequestReusePercentagePointDelta
    && candidate.commonPrefixBytesDelta === expected.commonPrefixBytesDelta
    && candidate.estimatedCommonPrefixTokensDelta === expected.estimatedCommonPrefixTokensDelta
    && candidate.uncachedSuffixBytesDelta === expected.uncachedSuffixBytesDelta
}

function sameComparisonTransition(
  candidate: unknown,
  expected: PromptCacheBenchmarkComparisonTransition,
): boolean {
  if (!isRecord(candidate)
    || !hasExactKeys(candidate, [
      "scenarioId",
      "scenarioTitle",
      "fromStepId",
      "toStepId",
      "cacheEpochTransition",
      "baseline",
      "optimized",
      "delta",
    ])
    || candidate.scenarioId !== expected.scenarioId
    || candidate.scenarioTitle !== expected.scenarioTitle
    || candidate.fromStepId !== expected.fromStepId
    || candidate.toStepId !== expected.toStepId
    || candidate.cacheEpochTransition !== expected.cacheEpochTransition
    || !sameSnapshot(candidate.baseline, expected.baseline)
    || !sameSnapshot(candidate.optimized, expected.optimized)
    || !isRecord(candidate.delta)
    || !hasExactKeys(candidate.delta, [
      "commonPrefixBytes",
      "estimatedCommonPrefixTokens",
      "nextRequestReusePercentagePoints",
      "uncachedSuffixBytes",
    ])) return false
  return candidate.delta.commonPrefixBytes === expected.delta.commonPrefixBytes
    && candidate.delta.estimatedCommonPrefixTokens === expected.delta.estimatedCommonPrefixTokens
    && candidate.delta.nextRequestReusePercentagePoints === expected.delta.nextRequestReusePercentagePoints
    && candidate.delta.uncachedSuffixBytes === expected.delta.uncachedSuffixBytes
}

function sameSnapshot(candidate: unknown, expected: TransitionSnapshot): boolean {
  return isRecord(candidate)
    && hasExactKeys(candidate, [
      "commonPrefixBytes",
      "estimatedCommonPrefixTokens",
      "nextRequestReuseRatio",
      "nextRequestBytes",
      "uncachedSuffixBytes",
    ])
    && candidate.commonPrefixBytes === expected.commonPrefixBytes
    && candidate.estimatedCommonPrefixTokens === expected.estimatedCommonPrefixTokens
    && candidate.nextRequestReuseRatio === expected.nextRequestReuseRatio
    && candidate.nextRequestBytes === expected.nextRequestBytes
    && candidate.uncachedSuffixBytes === expected.uncachedSuffixBytes
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}

function hasExactKeys(value: Record<string, unknown>, expected: readonly string[]): boolean {
  const keys = Object.keys(value)
  return keys.length === expected.length && expected.every(key => Object.hasOwn(value, key))
}
