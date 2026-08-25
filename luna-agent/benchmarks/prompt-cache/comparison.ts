import {
  isPromptCacheBenchmarkResult,
  type PromptCacheBenchmarkResult,
  type PromptCacheBenchmarkScenario,
  type PromptCacheBenchmarkTransition,
} from "./benchmark.js"

export const promptCacheBenchmarkComparisonSchemaVersion = "luna.agent.prompt-cache-benchmark-comparison.v1" as const

export type PromptCacheBenchmarkComparisonTransition = {
  scenarioId: string
  scenarioTitle: string
  fromStepId: string
  toStepId: string
  baseline: TransitionSnapshot | null
  optimized: TransitionSnapshot | null
  delta: {
    commonPrefixBytes: number | null
    estimatedCommonPrefixTokens: number | null
    nextRequestReusePercentagePoints: number | null
  }
}

export type PromptCacheBenchmarkComparison = {
  schemaVersion: typeof promptCacheBenchmarkComparisonSchemaVersion
  baseline: PromptCacheBenchmarkResult
  optimized: PromptCacheBenchmarkResult
  summary: {
    comparableTransitionCount: number
    missingBaselineTransitionCount: number
    missingOptimizedTransitionCount: number
    functionalAssertionsPassed: boolean
    weightedNextRequestReusePercentagePointDelta: number
    commonPrefixBytesDelta: number
    estimatedCommonPrefixTokensDelta: number
  }
  transitions: PromptCacheBenchmarkComparisonTransition[]
}

type TransitionSnapshot = {
  commonPrefixBytes: number
  estimatedCommonPrefixTokens: number
  nextRequestReuseRatio: number
  nextRequestBytes: number | null
}

type IndexedTransition = {
  scenario: PromptCacheBenchmarkScenario
  transition: PromptCacheBenchmarkTransition
}

export function comparePromptCacheBenchmarks(
  baseline: PromptCacheBenchmarkResult,
  optimized: PromptCacheBenchmarkResult,
): PromptCacheBenchmarkComparison {
  const baselineTransitions = indexTransitions(baseline)
  const optimizedTransitions = indexTransitions(optimized)
  const keys = [
    ...baselineTransitions.keys(),
    ...[...optimizedTransitions.keys()].filter(key => !baselineTransitions.has(key)),
  ]
  const transitions = keys.map((key) => {
    const baselineEntry = baselineTransitions.get(key)
    const optimizedEntry = optimizedTransitions.get(key)
    const scenario = optimizedEntry?.scenario ?? baselineEntry?.scenario
    if (!scenario) throw new Error(`prompt_cache_benchmark_transition_missing:${key}`)
    const baselineSnapshot = baselineEntry ? snapshot(baselineEntry) : null
    const optimizedSnapshot = optimizedEntry ? snapshot(optimizedEntry) : null
    return {
      scenarioId: scenario.id,
      scenarioTitle: scenario.title,
      fromStepId: optimizedEntry?.transition.fromStepId ?? baselineEntry!.transition.fromStepId,
      toStepId: optimizedEntry?.transition.toStepId ?? baselineEntry!.transition.toStepId,
      baseline: baselineSnapshot,
      optimized: optimizedSnapshot,
      delta: {
        commonPrefixBytes: difference(optimizedSnapshot?.commonPrefixBytes, baselineSnapshot?.commonPrefixBytes),
        estimatedCommonPrefixTokens: difference(
          optimizedSnapshot?.estimatedCommonPrefixTokens,
          baselineSnapshot?.estimatedCommonPrefixTokens,
        ),
        nextRequestReusePercentagePoints: percentagePointDifference(
          optimizedSnapshot?.nextRequestReuseRatio,
          baselineSnapshot?.nextRequestReuseRatio,
        ),
      },
    }
  })
  return {
    schemaVersion: promptCacheBenchmarkComparisonSchemaVersion,
    baseline,
    optimized,
    summary: {
      comparableTransitionCount: transitions.filter(item => item.baseline && item.optimized).length,
      missingBaselineTransitionCount: transitions.filter(item => !item.baseline).length,
      missingOptimizedTransitionCount: transitions.filter(item => !item.optimized).length,
      functionalAssertionsPassed: baseline.summary.failedAssertionCount === 0
        && optimized.summary.failedAssertionCount === 0,
      weightedNextRequestReusePercentagePointDelta: percentagePointDifference(
        optimized.summary.weightedNextRequestReuseRatio,
        baseline.summary.weightedNextRequestReuseRatio,
      ) ?? 0,
      commonPrefixBytesDelta: optimized.summary.commonPrefixBytes - baseline.summary.commonPrefixBytes,
      estimatedCommonPrefixTokensDelta: optimized.summary.estimatedCommonPrefixTokens
        - baseline.summary.estimatedCommonPrefixTokens,
    },
    transitions,
  }
}

export function isPromptCacheBenchmarkComparison(value: unknown): value is PromptCacheBenchmarkComparison {
  return isRecord(value)
    && value.schemaVersion === promptCacheBenchmarkComparisonSchemaVersion
    && isPromptCacheBenchmarkResult(value.baseline)
    && isPromptCacheBenchmarkResult(value.optimized)
    && isRecord(value.summary)
    && typeof value.summary.functionalAssertionsPassed === "boolean"
    && typeof value.summary.weightedNextRequestReusePercentagePointDelta === "number"
    && Array.isArray(value.transitions)
    && value.transitions.every(transition => isRecord(transition)
      && typeof transition.scenarioId === "string"
      && typeof transition.fromStepId === "string"
      && typeof transition.toStepId === "string"
      && isRecord(transition.delta))
}

function indexTransitions(result: PromptCacheBenchmarkResult): Map<string, IndexedTransition> {
  return new Map(result.scenarios.flatMap(scenario => scenario.transitions.map(transition => [
    transitionKey(scenario.id, transition),
    { scenario, transition },
  ])))
}

function transitionKey(scenarioId: string, transition: PromptCacheBenchmarkTransition): string {
  return `${scenarioId}\0${transition.fromStepId}\0${transition.toStepId}`
}

function snapshot(entry: IndexedTransition): TransitionSnapshot {
  return {
    commonPrefixBytes: entry.transition.commonPrefixBytes,
    estimatedCommonPrefixTokens: entry.transition.estimatedCommonPrefixTokens,
    nextRequestReuseRatio: entry.transition.nextRequestReuseRatio,
    nextRequestBytes: entry.scenario.steps.find(step => step.id === entry.transition.toStepId)?.requestBytes ?? null,
  }
}

function difference(optimized: number | undefined, baseline: number | undefined): number | null {
  return optimized === undefined || baseline === undefined ? null : optimized - baseline
}

function percentagePointDifference(optimized: number | undefined, baseline: number | undefined): number | null {
  const delta = difference(optimized, baseline)
  return delta === null ? null : Number((delta * 100).toFixed(4))
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}
