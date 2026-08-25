import type {
  PromptCacheBenchmarkAssertion,
  PromptCacheBenchmarkResult,
  PromptCacheBenchmarkScenario,
} from "./benchmark.js"
import {
  isPromptCacheBenchmarkComparison,
  type PromptCacheBenchmarkComparison,
  type PromptCacheBenchmarkComparisonTransition,
} from "./comparison.js"

export const promptCacheReportPalettes = {
  light: {
    background: "#f3f6fb",
    surface: "#ffffff",
    surfaceMuted: "#e8eef7",
    text: "#172033",
    muted: "#475569",
    border: "#94a3b8",
    accent: "#1d4ed8",
    success: "#166534",
    warning: "#854d0e",
    danger: "#b91c1c",
    focus: "#1d4ed8",
  },
  dark: {
    background: "#08111f",
    surface: "#111c2e",
    surfaceMuted: "#1e293b",
    text: "#f8fafc",
    muted: "#cbd5e1",
    border: "#64748b",
    accent: "#93c5fd",
    success: "#86efac",
    warning: "#fde68a",
    danger: "#fca5a5",
    focus: "#bfdbfe",
  },
} as const

export type PromptCacheBenchmarkReportData = PromptCacheBenchmarkResult | PromptCacheBenchmarkComparison

/** 将 benchmark JSON 渲染为不含脚本、字体、图片或网络依赖的单文件报告。 */
export function renderPromptCacheBenchmarkReport(data: PromptCacheBenchmarkReportData): string {
  const comparison = isPromptCacheBenchmarkComparison(data) ? data : undefined
  const primary = comparison?.optimized ?? data as PromptCacheBenchmarkResult
  const title = comparison
    ? `Agent Prompt Cache 基准：${comparison.baseline.benchmark.checkoutLabel} → ${comparison.optimized.benchmark.checkoutLabel}`
    : `Agent Prompt Cache 基准：${primary.benchmark.checkoutLabel}`
  const content = comparison ? renderComparison(comparison) : renderSingle(primary)
  return `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="color-scheme" content="light dark">
  <meta name="generator" content="Luna DevOps Agent Prompt Cache Benchmark">
  <title>${escapeHTML(title)}</title>
  <style>${reportStyles()}</style>
</head>
<body>
  <a class="skip-link" href="#main-content">跳到主要内容</a>
  <header class="page-header">
    <div class="shell header-layout">
      <div>
        <p class="eyebrow">Luna DevOps · Deterministic Benchmark</p>
        <h1>${escapeHTML(title)}</h1>
        <p class="lede">衡量序列化请求的稳定前缀，并用功能不变量守住行为等价。</p>
      </div>
      ${statusPill(primary.summary.failedAssertionCount === 0 && (comparison?.baseline.summary.failedAssertionCount ?? 0) === 0)}
    </div>
  </header>
  <main id="main-content" class="shell main" tabindex="-1">
    <aside class="notice" aria-labelledby="measurement-boundary-title">
      <h2 id="measurement-boundary-title">测量边界</h2>
      <p>${escapeHTML(primary.benchmark.disclaimer)}</p>
    </aside>
    ${content}
    <section class="panel raw-data" aria-labelledby="raw-data-title">
      <details>
        <summary id="raw-data-title">查看嵌入的结构化 JSON</summary>
        <pre><code>${escapeHTML(JSON.stringify(data, null, 2))}</code></pre>
      </details>
    </section>
  </main>
  <footer class="page-footer">
    <div class="shell">
      <p>Schema：<code>${escapeHTML(data.schemaVersion)}</code></p>
      <p>报告不含外网资源；深浅色跟随系统，打印时使用高对比浅色版。</p>
    </div>
  </footer>
</body>
</html>`
}

function renderSingle(result: PromptCacheBenchmarkResult): string {
  return `
    <section aria-labelledby="overview-title">
      <div class="section-heading">
        <div>
          <p class="eyebrow">Overview</p>
          <h2 id="overview-title">结果总览</h2>
        </div>
        <p class="section-note">Checkout：<code>${escapeHTML(result.benchmark.checkoutLabel)}</code></p>
      </div>
      <div class="metric-grid">
        ${metricCard("场景", formatInteger(result.summary.scenarioCount), "固定 fixture")}
        ${metricCard("相邻请求", formatInteger(result.summary.transitionCount), "逐对比较")}
        ${metricCard("加权前缀占比", formatPercent(result.summary.weightedNextRequestReuseRatio), "相对于后一个请求")}
        ${metricCard("估算公共前缀", formatInteger(result.summary.estimatedCommonPrefixTokens), "约每 4 UTF-8 字节 / Token")}
        ${metricCard("未复用后缀", formatInteger(result.summary.uncachedSuffixBytes), "后请求字节减公共前缀")}
        ${metricCard("Cache epoch 失效", formatInteger(result.summary.cacheEpochInvalidationTransitionCount), "压缩边界单独标记")}
      </div>
      ${renderEpochLayers(result)}
      <div class="source-proof" aria-label="Benchmark 来源校验">
        ${sourceIdentity("Source", result)}
        ${digestIdentity("Harness", result.benchmark.harnessDigest)}
      </div>
    </section>
    ${methodology()}
    <section aria-labelledby="scenarios-title">
      <div class="section-heading">
        <div>
          <p class="eyebrow">Scenarios</p>
          <h2 id="scenarios-title">场景明细</h2>
        </div>
      </div>
      <div class="scenario-stack">
        ${result.scenarios.map(renderScenario).join("\n")}
      </div>
    </section>`
}

function renderComparison(comparison: PromptCacheBenchmarkComparison): string {
  const { baseline, optimized, summary } = comparison
  const transitionsByScenario = groupBy(comparison.transitions, transition => transition.scenarioId)
  return `
    <section aria-labelledby="comparison-overview-title">
      <div class="section-heading">
        <div>
          <p class="eyebrow">Comparison</p>
          <h2 id="comparison-overview-title">Checkout 对比</h2>
        </div>
        <p class="section-note">${escapeHTML(baseline.benchmark.checkoutLabel)} → ${escapeHTML(optimized.benchmark.checkoutLabel)}</p>
      </div>
      <div class="metric-grid">
        ${metricCard("Baseline 前缀占比", formatPercent(baseline.summary.weightedNextRequestReuseRatio), baseline.benchmark.checkoutLabel)}
        ${metricCard("Optimized 前缀占比", formatPercent(optimized.summary.weightedNextRequestReuseRatio), optimized.benchmark.checkoutLabel)}
        ${metricCard("占比变化", formatSignedDecimal(summary.weightedNextRequestReusePercentagePointDelta, " pp"), deltaMeaning(summary.weightedNextRequestReusePercentagePointDelta))}
        ${metricCard("估算公共前缀变化", formatSigned(summary.estimatedCommonPrefixTokensDelta), "估算 Token")}
        ${metricCard("未复用后缀变化", formatSigned(summary.uncachedSuffixBytesDelta, " bytes"), "负值表示改善")}
      </div>
      ${renderEpochLayerComparison(baseline, optimized)}
      <div class="comparison-meta">
        <p><strong>${formatInteger(summary.comparableTransitionCount)}</strong> 组可比转换</p>
        <p><strong>${formatInteger(summary.missingBaselineTransitionCount)}</strong> 组缺 baseline</p>
        <p><strong>${formatInteger(summary.missingOptimizedTransitionCount)}</strong> 组缺 optimized</p>
        <p><strong>${formatInteger(baseline.summary.cacheEpochInvalidationTransitionCount)}</strong> 组 cache epoch 失效边界</p>
      </div>
      <div class="source-proof" aria-label="Baseline 与 optimized 来源校验">
        ${sourceIdentity("Baseline", baseline)}
        ${sourceIdentity("Optimized", optimized)}
        ${digestIdentity("共享 Harness", baseline.benchmark.harnessDigest)}
      </div>
    </section>
    ${methodology()}
    <section aria-labelledby="transition-comparison-title">
      <div class="section-heading">
        <div>
          <p class="eyebrow">Transitions</p>
          <h2 id="transition-comparison-title">逐场景转换</h2>
        </div>
      </div>
      <div class="scenario-stack">
        ${[...transitionsByScenario.values()].map(renderComparisonScenario).join("\n")}
      </div>
    </section>
    <section aria-labelledby="assertion-comparison-title">
      <div class="section-heading">
        <div>
          <p class="eyebrow">Functional invariants</p>
          <h2 id="assertion-comparison-title">功能等价断言</h2>
        </div>
      </div>
      <div class="checkout-grid">
        ${renderCheckoutAssertions(baseline)}
        ${renderCheckoutAssertions(optimized)}
      </div>
    </section>`
}

function methodology(): string {
  return `<section class="panel method" aria-labelledby="method-title">
      <div>
        <p class="eyebrow">Method</p>
        <h2 id="method-title">口径与复现原则</h2>
      </div>
      <dl class="definition-grid">
        <div><dt>序列化</dt><dd>生产 OpenAI-compatible chat/completions 流式请求体的 <code>JSON.stringify</code> 结果。</dd></div>
        <div><dt>公共前缀</dt><dd>相邻请求从第一个 UTF-8 字节开始连续相同的最长长度。</dd></div>
        <div><dt>Token 估算</dt><dd><code>ceil(commonPrefixBytes / 4)</code>，仅用于稳定的 checkout 间相对比较。</dd></div>
        <div><dt>未复用后缀</dt><dd><code>nextRequestBytes - commonPrefixBytes</code>；无需 Provider Tokenizer 即可并列观察新增或改写的请求负担。</dd></div>
        <div><dt>Cache epoch</dt><dd>压缩替换历史时明确标记为失效边界，保留原始指标但与摘要持久化后的同 epoch 复用区分解读。</dd></div>
        <div><dt>功能门禁</dt><dd>校验消息事实、工具 Schema、会话归属和压缩摘要不变量；失败时 runner 非零退出。</dd></div>
      </dl>
    </section>`
}

function renderEpochLayers(result: PromptCacheBenchmarkResult): string {
  const withinEpoch = cacheEpochSummary(result, "within_epoch")
  const invalidation = cacheEpochSummary(result, "cache_epoch_invalidation")
  return `<div class="epoch-summary-grid" aria-label="Cache epoch 分层指标">
      ${epochSummaryCard("同一 cache epoch", withinEpoch, "摘要与历史前缀可继续复用")}
      ${epochSummaryCard("Cache epoch 失效边界", invalidation, "压缩改写历史，原始指标仍计入总览")}
    </div>`
}

function renderEpochLayerComparison(
  baseline: PromptCacheBenchmarkResult,
  optimized: PromptCacheBenchmarkResult,
): string {
  const baselineWithin = cacheEpochSummary(baseline, "within_epoch")
  const optimizedWithin = cacheEpochSummary(optimized, "within_epoch")
  const baselineInvalidation = cacheEpochSummary(baseline, "cache_epoch_invalidation")
  const optimizedInvalidation = cacheEpochSummary(optimized, "cache_epoch_invalidation")
  return `<div class="epoch-summary-grid" aria-label="Baseline 与 optimized 的 cache epoch 分层对比">
      ${epochComparisonCard("同一 cache epoch", baselineWithin, optimizedWithin)}
      ${epochComparisonCard("Cache epoch 失效边界", baselineInvalidation, optimizedInvalidation)}
    </div>`
}

type CacheEpochSummary = {
  transitionCount: number
  commonPrefixBytes: number
  nextRequestBytes: number
  uncachedSuffixBytes: number
  weightedNextRequestReuseRatio: number
}

function cacheEpochSummary(
  result: PromptCacheBenchmarkResult,
  cacheEpochTransition: PromptCacheBenchmarkComparisonTransition["cacheEpochTransition"],
): CacheEpochSummary {
  const transitions = result.scenarios.flatMap(scenario => scenario.transitions)
    .filter(transition => transition.cacheEpochTransition === cacheEpochTransition)
  const commonPrefixBytes = transitions.reduce((total, transition) => total + transition.commonPrefixBytes, 0)
  const uncachedSuffixBytes = transitions.reduce((total, transition) => total + transition.uncachedSuffixBytes, 0)
  const nextRequestBytes = commonPrefixBytes + uncachedSuffixBytes
  return {
    transitionCount: transitions.length,
    commonPrefixBytes,
    nextRequestBytes,
    uncachedSuffixBytes,
    weightedNextRequestReuseRatio: nextRequestBytes > 0 ? commonPrefixBytes / nextRequestBytes : 0,
  }
}

function epochSummaryCard(label: string, summary: CacheEpochSummary, detail: string): string {
  return `<article class="epoch-summary"><p class="eyebrow">${escapeHTML(label)}</p><strong>${formatPercent(summary.weightedNextRequestReuseRatio)}</strong><span>${formatInteger(summary.transitionCount)} 组转换 · 未复用 ${formatInteger(summary.uncachedSuffixBytes)} / ${formatInteger(summary.nextRequestBytes)} bytes</span><small>${escapeHTML(detail)}</small></article>`
}

function epochComparisonCard(
  label: string,
  baseline: CacheEpochSummary,
  optimized: CacheEpochSummary,
): string {
  const reuseDelta = (optimized.weightedNextRequestReuseRatio - baseline.weightedNextRequestReuseRatio) * 100
  const uncachedDelta = optimized.uncachedSuffixBytes - baseline.uncachedSuffixBytes
  return `<article class="epoch-summary"><p class="eyebrow">${escapeHTML(label)}</p><strong>${formatPercent(baseline.weightedNextRequestReuseRatio)} → ${formatPercent(optimized.weightedNextRequestReuseRatio)}</strong><span>占比 ${formatSignedDecimal(reuseDelta, " pp")} · 未复用后缀 ${formatSigned(uncachedDelta)} bytes</span><small>Baseline ${formatInteger(baseline.transitionCount)} 组 / Optimized ${formatInteger(optimized.transitionCount)} 组；未复用负值更好</small></article>`
}

function renderScenario(scenario: PromptCacheBenchmarkScenario): string {
  const allPassed = scenario.assertions.every(assertion => assertion.passed)
  const regionId = `scenario-${safeID(scenario.id)}`
  return `<article class="panel scenario" aria-labelledby="${regionId}-title">
      <div class="scenario-header">
        <div>
          <p class="eyebrow">${escapeHTML(scenario.id)}</p>
          <h3 id="${regionId}-title">${escapeHTML(scenario.title)}</h3>
          <p>${escapeHTML(scenario.description)}</p>
        </div>
        ${statusPill(allPassed)}
      </div>
      ${renderTransitionTable(scenario, regionId)}
      ${renderStepTable(scenario, regionId)}
      ${renderAssertions(scenario.assertions, `${regionId}-assertions`)}
    </article>`
}

function renderTransitionTable(scenario: PromptCacheBenchmarkScenario, id: string): string {
  if (!scenario.transitions.length) return `<p class="empty">该场景没有相邻请求可比较。</p>`
  return `<div class="table-region" role="region" aria-labelledby="${id}-transition-caption" tabindex="0">
        <table>
          <caption id="${id}-transition-caption">相邻请求最长公共前缀</caption>
          <thead><tr><th scope="col">转换</th><th scope="col">缓存阶段</th><th scope="col">公共前缀字节</th><th scope="col">估算 Token</th><th scope="col">后请求占比</th><th scope="col">未复用后缀</th></tr></thead>
          <tbody>${scenario.transitions.map(transition => `<tr>
            <th scope="row"><code>${escapeHTML(transition.fromStepId)}</code> → <code>${escapeHTML(transition.toStepId)}</code></th>
            <td>${cacheEpochBadge(transition.cacheEpochTransition)}</td>
            <td>${formatInteger(transition.commonPrefixBytes)}</td>
            <td>${formatInteger(transition.estimatedCommonPrefixTokens)}</td>
            <td>${ratioMeter(transition.nextRequestReuseRatio, `${scenario.title} ${transition.fromStepId} 到 ${transition.toStepId} 的后请求前缀占比`)}</td>
            <td>${formatInteger(transition.uncachedSuffixBytes)} bytes</td>
          </tr>`).join("")}</tbody>
        </table>
      </div>`
}

function renderStepTable(scenario: PromptCacheBenchmarkScenario, id: string): string {
  return `<div class="table-region" role="region" aria-labelledby="${id}-step-caption" tabindex="0">
        <table>
          <caption id="${id}-step-caption">请求快照（不包含原始 Prompt）</caption>
          <thead><tr><th scope="col">步骤</th><th scope="col">字节</th><th scope="col">消息</th><th scope="col">工具</th><th scope="col">SHA-256</th></tr></thead>
          <tbody>${scenario.steps.map(step => `<tr>
            <th scope="row">${escapeHTML(step.label)}</th>
            <td>${formatInteger(step.requestBytes)}</td>
            <td>${formatInteger(step.messageCount)}</td>
            <td>${formatInteger(step.toolCount)}</td>
            <td><code class="hash" aria-label="SHA-256：${escapeHTML(step.requestSha256)}" title="${escapeHTML(step.requestSha256)}">${escapeHTML(step.requestSha256.slice(0, 12))}…</code></td>
          </tr>`).join("")}</tbody>
        </table>
      </div>`
}

function renderComparisonScenario(transitions: PromptCacheBenchmarkComparisonTransition[]): string {
  const first = transitions[0]
  if (!first) return ""
  const id = `comparison-${safeID(first.scenarioId)}`
  return `<article class="panel scenario" aria-labelledby="${id}-title">
      <div class="scenario-header">
        <div>
          <p class="eyebrow">${escapeHTML(first.scenarioId)}</p>
          <h3 id="${id}-title">${escapeHTML(first.scenarioTitle)}</h3>
        </div>
      </div>
      <div class="table-region" role="region" aria-labelledby="${id}-caption" tabindex="0">
        <table>
          <caption id="${id}-caption">Baseline 与 optimized 的相邻请求前缀对比</caption>
          <thead><tr><th scope="col">转换</th><th scope="col">缓存阶段</th><th scope="col">Baseline</th><th scope="col">Optimized</th><th scope="col">变化</th></tr></thead>
          <tbody>${transitions.map(transition => `<tr>
            <th scope="row"><code>${escapeHTML(transition.fromStepId)}</code> → <code>${escapeHTML(transition.toStepId)}</code></th>
            <td>${cacheEpochBadge(transition.cacheEpochTransition)}</td>
            <td>${transitionSnapshot(transition.baseline)}</td>
            <td>${transitionSnapshot(transition.optimized)}</td>
            <td>${deltaCell(transition.delta.nextRequestReusePercentagePoints, transition.delta.commonPrefixBytes, transition.delta.uncachedSuffixBytes)}</td>
          </tr>`).join("")}</tbody>
        </table>
      </div>
    </article>`
}

function renderCheckoutAssertions(result: PromptCacheBenchmarkResult): string {
  const assertions = result.scenarios.flatMap(scenario => scenario.assertions.map(assertion => ({
    ...assertion,
    scenarioTitle: scenario.title,
  })))
  const allPassed = assertions.every(assertion => assertion.passed)
  return `<article class="panel assertion-checkout">
      <div class="scenario-header">
        <div><p class="eyebrow">Checkout</p><h3>${escapeHTML(result.benchmark.checkoutLabel)}</h3></div>
        ${statusPill(allPassed)}
      </div>
      <ul class="assertion-list">${assertions.map(assertion => `<li class="${assertion.passed ? "pass" : "fail"}">
        <span aria-hidden="true">${assertion.passed ? "✓" : "×"}</span>
        <span><strong>${escapeHTML(assertion.scenarioTitle)}：</strong>${escapeHTML(assertion.description)}</span>
      </li>`).join("")}</ul>
    </article>`
}

function renderAssertions(assertions: PromptCacheBenchmarkAssertion[], id: string): string {
  return `<section class="assertions" aria-labelledby="${id}">
      <h4 id="${id}">功能不变量</h4>
      <ul class="assertion-list">${assertions.map(assertion => `<li class="${assertion.passed ? "pass" : "fail"}">
        <span aria-hidden="true">${assertion.passed ? "✓" : "×"}</span>
        <span>${escapeHTML(assertion.description)}</span>
      </li>`).join("")}</ul>
    </section>`
}

function transitionSnapshot(snapshot: PromptCacheBenchmarkComparisonTransition["baseline"]): string {
  if (!snapshot) return `<span class="not-available">无对应数据</span>`
  return `<span class="stacked-value"><strong>${formatPercent(snapshot.nextRequestReuseRatio)}</strong><span>${formatInteger(snapshot.commonPrefixBytes)} bytes · 约 ${formatInteger(snapshot.estimatedCommonPrefixTokens)} Token</span><span>未复用 ${formatInteger(snapshot.uncachedSuffixBytes)} bytes</span></span>`
}

function deltaCell(percentagePoints: number, bytes: number, uncachedSuffixBytes: number): string {
  const tone = percentagePoints > 0 ? "positive" : percentagePoints < 0 ? "negative" : "neutral"
  return `<span class="delta ${tone}"><strong>${escapeHTML(deltaMeaning(percentagePoints))} ${formatSignedDecimal(percentagePoints, " pp")}</strong><span>公共前缀 ${formatSigned(bytes)} bytes</span><span>未复用后缀 ${formatSigned(uncachedSuffixBytes)} bytes（负值更好）</span></span>`
}

function cacheEpochBadge(value: PromptCacheBenchmarkComparisonTransition["cacheEpochTransition"]): string {
  const invalidated = value === "cache_epoch_invalidation"
  return `<span class="epoch ${invalidated ? "invalidation" : "within"}">${invalidated ? "Cache epoch 失效" : "同一 cache epoch"}</span>`
}

function metricCard(label: string, value: string, detail: string): string {
  return `<article class="metric"><h3>${escapeHTML(label)}</h3><p class="metric-value">${escapeHTML(value)}</p><p>${escapeHTML(detail)}</p></article>`
}

function sourceIdentity(label: string, result: PromptCacheBenchmarkResult): string {
  return `<p><strong>${escapeHTML(label)}</strong><span>Revision <code title="${escapeHTML(result.benchmark.sourceRevision)}">${escapeHTML(shortDigest(result.benchmark.sourceRevision))}</code></span><span>Implementation <code title="${escapeHTML(result.benchmark.implementationDigest)}">${escapeHTML(shortDigest(result.benchmark.implementationDigest))}</code></span></p>`
}

function digestIdentity(label: string, digest: string): string {
  return `<p><strong>${escapeHTML(label)}</strong><span><code title="${escapeHTML(digest)}">${escapeHTML(shortDigest(digest))}</code></span></p>`
}

function shortDigest(value: string): string {
  return `${value.slice(0, 12)}…`
}

function statusPill(passed: boolean): string {
  return `<span class="status ${passed ? "pass" : "fail"}"><span aria-hidden="true">${passed ? "✓" : "×"}</span>${passed ? "功能断言通过" : "存在断言失败"}</span>`
}

function ratioMeter(value: number, label: string): string {
  const bounded = Math.max(0, Math.min(1, value))
  return `<span class="meter-layout"><meter min="0" max="1" value="${bounded}" aria-label="${escapeHTML(label)}">${formatPercent(bounded)}</meter><span>${formatPercent(value)}</span></span>`
}

function groupBy<T, K>(items: T[], key: (item: T) => K): Map<K, T[]> {
  const groups = new Map<K, T[]>()
  for (const item of items) groups.set(key(item), [...(groups.get(key(item)) ?? []), item])
  return groups
}

function formatInteger(value: number): string {
  const sign = value < 0 ? "-" : ""
  return `${sign}${Math.abs(Math.trunc(value)).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ",")}`
}

function formatPercent(ratio: number): string {
  return `${(ratio * 100).toFixed(1)}%`
}

function formatSigned(value: number, suffix = ""): string {
  const formatted = Number.isInteger(value) ? formatInteger(value) : Math.abs(value).toFixed(2)
  return `${value > 0 ? "+" : value < 0 ? "-" : ""}${value < 0 ? formatted.replace(/^-/, "") : formatted}${suffix}`
}

function formatSignedDecimal(value: number, suffix = ""): string {
  return `${value > 0 ? "+" : value < 0 ? "-" : ""}${Math.abs(value).toFixed(2)}${suffix}`
}

function deltaMeaning(value: number): string {
  return value > 0 ? "提升" : value < 0 ? "下降" : "持平"
}

function safeID(value: string): string {
  return value.replace(/[^a-zA-Z0-9_-]/g, "-")
}

function escapeHTML(value: string): string {
  return value.replace(/[&<>"']/g, character => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    "\"": "&quot;",
    "'": "&#39;",
  })[character]!)
}

function reportStyles(): string {
  const light = promptCacheReportPalettes.light
  const dark = promptCacheReportPalettes.dark
  return `
    :root {
      color-scheme: light dark;
      --background: ${light.background};
      --surface: ${light.surface};
      --surface-muted: ${light.surfaceMuted};
      --text: ${light.text};
      --muted: ${light.muted};
      --border: ${light.border};
      --accent: ${light.accent};
      --success: ${light.success};
      --warning: ${light.warning};
      --danger: ${light.danger};
      --focus: ${light.focus};
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      font-size: 16px;
      line-height: 1.55;
    }
    * { box-sizing: border-box; }
    html { scroll-behavior: smooth; }
    body { margin: 0; background: var(--background); color: var(--text); }
    a { color: var(--accent); }
    code { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; overflow-wrap: anywhere; }
    h1, h2, h3, h4, p { margin-block-start: 0; }
    h1 { margin-block-end: .75rem; font-size: clamp(1.75rem, 5vw, 3rem); line-height: 1.12; letter-spacing: -.025em; }
    h2 { margin-block-end: .5rem; font-size: clamp(1.35rem, 3vw, 2rem); line-height: 1.2; }
    h3 { margin-block-end: .4rem; font-size: 1.1rem; line-height: 1.3; }
    h4 { margin-block-end: .75rem; font-size: 1rem; }
    .shell { width: min(100% - 2rem, 76rem); margin-inline: auto; }
    .skip-link { position: fixed; z-index: 100; inset: .75rem auto auto .75rem; padding: .65rem 1rem; border-radius: .5rem; background: var(--surface); color: var(--text); transform: translateY(-180%); }
    .skip-link:focus { transform: translateY(0); }
    :focus-visible { outline: .2rem solid var(--focus); outline-offset: .2rem; }
    .page-header { padding-block: clamp(2rem, 6vw, 4.5rem); background: var(--surface); border-block-end: 1px solid var(--border); }
    .header-layout { display: flex; align-items: flex-start; justify-content: space-between; gap: 1.5rem; flex-wrap: wrap; }
    .lede { max-width: 48rem; margin-block-end: 0; color: var(--muted); font-size: 1.05rem; }
    .eyebrow { margin-block-end: .45rem; color: var(--accent); font-size: .78rem; font-weight: 750; letter-spacing: .1em; text-transform: uppercase; }
    .main { display: grid; grid-template-columns: minmax(0, 1fr); gap: clamp(2rem, 5vw, 3.5rem); padding-block: clamp(1.5rem, 5vw, 3.5rem); }
    .main > * { min-width: 0; }
    .notice { padding: 1rem 1.25rem; border-inline-start: .35rem solid var(--warning); border-radius: .25rem .75rem .75rem .25rem; background: var(--surface); }
    .notice h2 { font-size: 1rem; }
    .notice p { margin-block-end: 0; color: var(--muted); }
    .section-heading { display: flex; align-items: end; justify-content: space-between; gap: 1rem; margin-block-end: 1rem; flex-wrap: wrap; }
    .section-note { margin-block-end: 0; color: var(--muted); }
    .metric-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 13rem), 1fr)); gap: 1rem; }
    .metric, .panel { border-radius: 1rem; background: var(--surface); }
    .metric { padding: 1.25rem; }
    .metric h3 { color: var(--muted); font-size: .9rem; font-weight: 650; }
    .metric p { margin-block-end: 0; color: var(--muted); }
    .metric .metric-value { margin-block: .3rem; color: var(--text); font-size: clamp(1.55rem, 4vw, 2.25rem); font-weight: 760; font-variant-numeric: tabular-nums; }
    .epoch-summary-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 22rem), 1fr)); gap: 1rem; margin-block-start: 1rem; }
    .epoch-summary { display: grid; gap: .25rem; padding: 1rem 1.25rem; border-radius: 1rem; background: var(--surface); }
    .epoch-summary p, .epoch-summary strong, .epoch-summary span, .epoch-summary small { margin: 0; }
    .epoch-summary strong { font-size: 1.25rem; font-variant-numeric: tabular-nums; }
    .epoch-summary span, .epoch-summary small { color: var(--muted); }
    .panel { padding: clamp(1rem, 3vw, 1.5rem); }
    .method { display: grid; gap: 1.25rem; }
    .definition-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 16rem), 1fr)); gap: 1rem; margin: 0; }
    .definition-grid div { padding: 1rem; border-radius: .75rem; background: var(--surface-muted); }
    .definition-grid dt { margin-block-end: .3rem; font-weight: 750; }
    .definition-grid dd { margin: 0; color: var(--muted); }
    .scenario-stack { display: grid; grid-template-columns: minmax(0, 1fr); gap: 1rem; }
    .scenario-stack > * { min-width: 0; }
    .scenario-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 1rem; margin-block-end: 1rem; flex-wrap: wrap; }
    .scenario-header p:last-child { max-width: 52rem; margin-block-end: 0; color: var(--muted); }
    .status { display: inline-flex; align-items: center; gap: .4rem; min-height: 2rem; padding: .3rem .7rem; border: 1px solid currentColor; border-radius: 999px; font-size: .85rem; font-weight: 750; white-space: nowrap; }
    .status.pass, .assertion-list .pass { color: var(--success); }
    .status.fail, .assertion-list .fail { color: var(--danger); }
    .table-region { width: 100%; min-width: 0; max-width: 100%; margin-block: 1rem; overflow-x: auto; overscroll-behavior-inline: contain; border: 1px solid var(--border); border-radius: .75rem; }
    table { width: 100%; min-width: 42rem; border-collapse: collapse; background: var(--surface); font-variant-numeric: tabular-nums; }
    caption { padding: .8rem 1rem; color: var(--muted); font-weight: 650; text-align: left; }
    th, td { padding: .75rem 1rem; border-block-start: 1px solid var(--border); text-align: left; vertical-align: middle; }
    thead th { background: var(--surface-muted); font-size: .86rem; }
    tbody th { font-weight: 650; }
    .hash { white-space: nowrap; }
    .meter-layout { display: grid; grid-template-columns: minmax(6rem, 1fr) auto; align-items: center; gap: .65rem; min-width: 11rem; }
    meter { width: 100%; height: .75rem; accent-color: var(--accent); }
    meter::-webkit-meter-bar { border: 0; border-radius: 999px; background: var(--surface-muted); }
    meter::-webkit-meter-optimum-value { border-radius: 999px; background: var(--accent); }
    .assertions { margin-block-start: 1.25rem; }
    .assertion-list { display: grid; gap: .55rem; margin: 0; padding: 0; list-style: none; }
    .assertion-list li { display: grid; grid-template-columns: 1.25rem 1fr; gap: .35rem; }
    .assertion-list li span:last-child { color: var(--text); }
    .comparison-meta { display: flex; gap: 1rem 2rem; margin-block-start: 1rem; color: var(--muted); flex-wrap: wrap; }
    .comparison-meta p { margin: 0; }
    .source-proof { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 18rem), 1fr)); gap: .75rem; margin-block-start: 1rem; }
    .source-proof p { display: grid; gap: .2rem; margin: 0; padding: .8rem 1rem; border-radius: .75rem; background: var(--surface); color: var(--muted); }
    .source-proof strong { color: var(--text); }
    .source-proof span { overflow-wrap: anywhere; }
    .checkout-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 25rem), 1fr)); gap: 1rem; }
    .stacked-value, .delta { display: grid; gap: .1rem; }
    .stacked-value span, .delta span { color: var(--muted); font-size: .82rem; white-space: nowrap; }
    .epoch { display: inline-flex; align-items: center; min-height: 1.75rem; padding: .2rem .55rem; border: 1px solid currentColor; border-radius: 999px; font-size: .78rem; font-weight: 700; white-space: nowrap; }
    .epoch.invalidation { color: var(--warning); }
    .epoch.within { color: var(--muted); }
    .delta.positive strong { color: var(--success); }
    .delta.negative strong { color: var(--danger); }
    .not-available { color: var(--muted); }
    .raw-data summary { cursor: pointer; font-weight: 700; }
    .raw-data pre { width: 100%; min-width: 0; max-width: 100%; max-height: 32rem; margin-block: 1rem 0; padding: 1rem; overflow: auto; border-radius: .75rem; background: var(--surface-muted); font-size: .82rem; }
    .page-footer { padding-block: 1.5rem 2.5rem; border-block-start: 1px solid var(--border); color: var(--muted); }
    .page-footer .shell { display: flex; justify-content: space-between; gap: .75rem 2rem; flex-wrap: wrap; }
    .page-footer p { margin: 0; }
    .empty { color: var(--muted); }
    @media (prefers-color-scheme: dark) {
      :root {
        --background: ${dark.background};
        --surface: ${dark.surface};
        --surface-muted: ${dark.surfaceMuted};
        --text: ${dark.text};
        --muted: ${dark.muted};
        --border: ${dark.border};
        --accent: ${dark.accent};
        --success: ${dark.success};
        --warning: ${dark.warning};
        --danger: ${dark.danger};
        --focus: ${dark.focus};
      }
    }
    @media (max-width: 40rem) {
      .shell { width: min(100% - 1.25rem, 76rem); }
      .page-header { padding-block: 1.75rem; }
      .metric, .panel { border-radius: .75rem; }
      .status { white-space: normal; }
    }
    @media (prefers-reduced-motion: reduce) {
      html { scroll-behavior: auto; }
      *, *::before, *::after { scroll-behavior: auto !important; transition-duration: .01ms !important; animation-duration: .01ms !important; animation-iteration-count: 1 !important; }
    }
    @media print {
      @page { size: A4; margin: 12mm; }
      :root {
        color-scheme: light;
        --background: #ffffff;
        --surface: #ffffff;
        --surface-muted: #f1f5f9;
        --text: #000000;
        --muted: #334155;
        --border: #64748b;
        --accent: #1e3a8a;
        --success: #14532d;
        --warning: #713f12;
        --danger: #991b1b;
        --focus: #1e3a8a;
      }
      * { print-color-adjust: exact; -webkit-print-color-adjust: exact; }
      body { background: #ffffff; font-size: 10pt; }
      .skip-link, .raw-data { display: none !important; }
      .shell { width: 100%; }
      .page-header, .main, .page-footer { padding-block: 8mm; }
      .main { gap: 8mm; }
      .metric-grid { grid-template-columns: repeat(4, 1fr); }
      .metric, .panel, .notice { break-inside: avoid; box-shadow: none; }
      .scenario { break-before: auto; }
      .table-region { overflow: visible; }
      table { min-width: 0; font-size: 8.5pt; }
      th, td { padding: 4pt 5pt; }
      thead { display: table-header-group; }
      tr { break-inside: avoid; }
    }
  `
}
