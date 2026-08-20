import type {
  ToolRetrievalMatch,
  ToolRetrievalQuery,
  ToolRetrievalReason,
  ToolRetrievalResult,
} from "../contracts.js"
import { BM25Index, type RankedOperation } from "./bm25.js"
import { buildToolRetrievalDocuments, type RetrievableTool, type ToolDocumentKind, type ToolRetrievalDocuments } from "./documents.js"
import { type EmbeddingProvider, InMemoryToolVectorIndex } from "./embeddings.js"
import { UnicodeLexicalTokenizer, type LexicalTokenizer } from "./tokenizer.js"

export type ToolRerankCandidate = {
  operationId: string
  resourceTypes: string[]
  action: string
  intents: string[]
  useWhen: string[]
  avoidWhen: string[]
  prerequisites: string[]
  successEvidence: string[]
}

export interface ToolReranker {
  rerank(
    query: ToolRetrievalQuery,
    candidates: ToolRerankCandidate[],
    signal?: AbortSignal,
  ): Promise<Array<{ operationId: string, score: number }>>
}

export type ToolRetrievalOptions = {
  limit?: number
  stickyOperationIds?: string[]
}

export type HybridToolRetrieverOptions = {
  tokenizer?: LexicalTokenizer
  embeddingProvider?: EmbeddingProvider
  reranker?: ToolReranker
  rrfK?: number
}

type Candidate = ToolRetrievalMatch & {
  workflowPriority: number
}

type RecallLists = Partial<Record<"intent" | "parameters" | "workflow" | "lexical", RankedOperation[]>>

const defaultLimit = 8

export class HybridToolRetriever {
  private readonly operations: Map<string, RetrievableTool>
  private readonly documents: Map<string, ToolRetrievalDocuments>
  private readonly bm25: BM25Index
  private readonly embeddingProvider: EmbeddingProvider | undefined
  private readonly reranker: ToolReranker | undefined
  private readonly rrfK: number
  private vectorIndex: InMemoryToolVectorIndex | undefined
  private vectorIndexBuild: Promise<InMemoryToolVectorIndex> | undefined

  constructor(operations: RetrievableTool[], options: HybridToolRetrieverOptions = {}) {
    const tokenizer = options.tokenizer ?? new UnicodeLexicalTokenizer()
    this.operations = new Map(operations.map(operation => [operation.operationId, operation]))
    this.documents = new Map(operations.map(operation => [
      operation.operationId,
      buildToolRetrievalDocuments(operation, tokenizer),
    ]))
    this.bm25 = new BM25Index([...this.documents.values()].map(item => item.lexical), tokenizer)
    this.embeddingProvider = options.embeddingProvider
    this.reranker = options.reranker
    this.rrfK = options.rrfK ?? 60
  }

  retrieveSync(query: ToolRetrievalQuery, options: ToolRetrievalOptions = {}): ToolRetrievalResult {
    if (!this.operations.size) return unavailable(query)
    const lexical = this.bm25.search(queryText(query), 30)
    return this.finalize(query, { lexical }, options, [
      this.embeddingProvider ? "embedding_async_required" : "embedding_unavailable",
      this.reranker ? "rerank_async_required" : "rerank_unavailable",
    ])
  }

  async retrieve(
    query: ToolRetrievalQuery,
    options: ToolRetrievalOptions = {},
    signal?: AbortSignal,
  ): Promise<ToolRetrievalResult> {
    if (!this.operations.size) return unavailable(query)
    const text = queryText(query)
    const lexical = this.bm25.search(text, 30)
    const degradedReasons: string[] = []
    let dense: RecallLists = {}

    if (!this.embeddingProvider) {
      degradedReasons.push("embedding_unavailable")
    } else {
      try {
        const vectorIndex = await this.getVectorIndex(signal)
        dense = await vectorIndex.search(text, this.embeddingProvider, undefined, signal)
      } catch {
        degradedReasons.push("embedding_unavailable")
      }
    }

    let candidates = this.fuseAndExpand(query, { ...dense, lexical }, options)
    if (!this.reranker) {
      degradedReasons.push("rerank_unavailable")
    } else if (candidates.length) {
      try {
        candidates = await this.applyReranker(query, candidates, signal)
      } catch {
        degradedReasons.push("rerank_unavailable")
      }
    }

    return resultFromCandidates(query, candidates, options.limit, dense.intent?.length ? "hybrid" : strategyFor(candidates), degradedReasons)
  }

  private finalize(
    query: ToolRetrievalQuery,
    recalls: RecallLists,
    options: ToolRetrievalOptions,
    degradedReasons: string[],
  ): ToolRetrievalResult {
    const candidates = this.fuseAndExpand(query, recalls, options)
    return resultFromCandidates(query, candidates, options.limit, strategyFor(candidates), degradedReasons)
  }

  private fuseAndExpand(
    query: ToolRetrievalQuery,
    recalls: RecallLists,
    options: ToolRetrievalOptions,
  ): Candidate[] {
    const candidates = reciprocalRankFusion(recalls, this.rrfK)
    const completed = new Set(query.completedOperations.filter(operationId => this.operations.has(operationId)))
    const sticky = unique(options.stickyOperationIds ?? []).filter(operationId => this.operations.has(operationId))
    const stickySet = new Set(sticky)

    if (query.pendingState) {
      for (const [operationId, candidate] of candidates) {
        const operation = this.operations.get(operationId)
        if (candidate.reasonCode === "goal_match" && operation && hasWriteSideEffect(operation) && !stickySet.has(operationId)) {
          candidates.delete(operationId)
        }
      }
    }

    for (const operationId of sticky) upsertWorkflowCandidate(candidates, operationId, "sticky_operation", 4)
    for (const operationId of unique([...completed, ...sticky])) {
      const operation = this.operations.get(operationId)
      if (!operation) continue
      if (operation.contract.verification.mode !== "response") {
        upsertWorkflowCandidate(candidates, operation.contract.verification.operationId, "required_verifier", 3)
      }
      for (const followup of operation.contract.followups) {
        upsertWorkflowCandidate(candidates, followup, "workflow_followup", 2)
      }
    }

    const strongestGoalCandidates = [...candidates.values()]
      .filter(candidate => candidate.reasonCode === "goal_match")
      .sort(compareCandidates)
      .slice(0, 1)
    for (const candidate of strongestGoalCandidates) {
      const operation = this.operations.get(candidate.operationId)
      if (!operation) continue
      // predecessors 同时承载“写操作前置读取”和“写后回读的反向关系”。
      // 读取工具不能因为反向关系把创建/修改工具提升到自己前面，否则一次详情查询会意外暴露写操作。
      candidate.missingPrerequisites = hasWriteSideEffect(operation)
        ? operation.contract.predecessors.filter((operationId) => {
            const predecessor = this.operations.get(operationId)
            return !completed.has(operationId) && predecessor !== undefined && !hasWriteSideEffect(predecessor)
          })
        : []
      for (const predecessor of candidate.missingPrerequisites) {
        upsertWorkflowCandidate(candidates, predecessor, "required_predecessor", 2)
      }
    }

    return [...candidates.values()].sort(compareCandidates)
  }

  private async applyReranker(
    query: ToolRetrievalQuery,
    candidates: Candidate[],
    signal?: AbortSignal,
  ): Promise<Candidate[]> {
    const rerankable = candidates.filter(candidate => candidate.reasonCode === "goal_match")
    const input = rerankable.map(candidate => {
      const contract = this.operations.get(candidate.operationId)!.contract
      return {
        operationId: candidate.operationId,
        resourceTypes: contract.resourceTypes,
        action: contract.action,
        intents: contract.intents,
        useWhen: contract.useWhen,
        avoidWhen: contract.avoidWhen,
        prerequisites: contract.prerequisites,
        successEvidence: contract.successEvidence,
      }
    })
    const reranked = await this.reranker!.rerank(query, input, signal)
    const scores = new Map<string, number>()
    for (const item of reranked) {
      if (!this.operations.has(item.operationId) || !Number.isFinite(item.score) || scores.has(item.operationId)) continue
      scores.set(item.operationId, item.score)
    }
    return candidates.map(candidate => ({
      ...candidate,
      relevance: candidate.reasonCode === "goal_match" ? (scores.get(candidate.operationId) ?? candidate.relevance) : candidate.relevance,
    })).sort(compareCandidates)
  }

  private async getVectorIndex(signal?: AbortSignal): Promise<InMemoryToolVectorIndex> {
    if (this.vectorIndex) return this.vectorIndex
    if (!this.embeddingProvider) throw new Error("ai.tool_embedding_unavailable")
    if (!this.vectorIndexBuild) {
      const vectorDocuments = [...this.documents.values()].flatMap(item => Object.values(item.vectors))
      this.vectorIndexBuild = InMemoryToolVectorIndex.build(this.embeddingProvider, vectorDocuments, signal)
        .then(index => {
          this.vectorIndex = index
          return index
        })
        .catch((error: unknown) => {
          this.vectorIndexBuild = undefined
          throw error
        })
    }
    return this.vectorIndexBuild
  }
}

function reciprocalRankFusion(recalls: RecallLists, k: number): Map<string, Candidate> {
  const candidates = new Map<string, Candidate>()
  for (const [kind, ranked] of Object.entries(recalls) as Array<[keyof RecallLists, RankedOperation[] | undefined]>) {
    ranked?.forEach((item, index) => {
      const rank = index + 1
      const current = candidates.get(item.operationId) ?? {
        operationId: item.operationId,
        relevance: 0,
        reasonCode: "goal_match" as const,
        missingPrerequisites: [],
        ranks: {},
        workflowPriority: 1,
      }
      current.relevance += 1 / (k + rank)
      current.ranks[kind] = rank
      candidates.set(item.operationId, current)
    })
  }
  return candidates
}

function upsertWorkflowCandidate(
  candidates: Map<string, Candidate>,
  operationId: string,
  reasonCode: ToolRetrievalReason,
  workflowPriority: number,
): void {
  const existing = candidates.get(operationId)
  if (existing) {
    if (workflowPriority > existing.workflowPriority) {
      existing.workflowPriority = workflowPriority
      existing.reasonCode = reasonCode
    }
    return
  }
  candidates.set(operationId, {
    operationId,
    relevance: 0,
    reasonCode,
    missingPrerequisites: [],
    ranks: {},
    workflowPriority,
  })
}

function resultFromCandidates(
  query: ToolRetrievalQuery,
  candidates: Candidate[],
  requestedLimit: number | undefined,
  strategy: ToolRetrievalResult["strategy"],
  degradedReasons: string[],
): ToolRetrievalResult {
  const limit = Math.max(1, Math.min(12, requestedLimit ?? defaultLimit))
  const required = candidates.filter(candidate => candidate.reasonCode === "required_verifier" || candidate.reasonCode === "required_predecessor")
  const sticky = candidates.filter(candidate => candidate.reasonCode === "sticky_operation")
  const goals = candidates.filter(candidate => candidate.reasonCode === "goal_match" || candidate.reasonCode === "ambiguous_candidate").slice(0, limit)
  const followups = candidates.filter(candidate => candidate.reasonCode === "workflow_followup")
  const selected = uniqueCandidates(required).slice(0, 12)
  const reserveGoal = goals.length > 0 && selected.length < 12 ? 1 : 0
  appendCandidates(selected, sticky, 12 - reserveGoal)
  appendCandidates(selected, goals, 12)
  appendCandidates(selected, followups, 12)
  const matches = selected.map(candidate => ({
    operationId: candidate.operationId,
    relevance: candidate.relevance,
    reasonCode: candidate.reasonCode,
    missingPrerequisites: candidate.missingPrerequisites,
    ranks: candidate.ranks,
  }))
  const reasons = unique(degradedReasons)
  return {
    query,
    matches,
    loadedOperationIds: matches.map(item => item.operationId),
    totalMatches: candidates.length,
    strategy,
    outcome: reasons.length ? "degraded" : "succeeded",
    ...(reasons.length ? { degradedReason: reasons.join(",") } : {}),
  }
}

function unavailable(query: ToolRetrievalQuery): ToolRetrievalResult {
  return {
    query,
    matches: [],
    loadedOperationIds: [],
    totalMatches: 0,
    strategy: "base_only",
    outcome: "unavailable",
    degradedReason: "no_allowed_operations",
  }
}

function compareCandidates(left: Candidate, right: Candidate): number {
  return right.workflowPriority - left.workflowPriority
    || right.relevance - left.relevance
    || compareOperationId(left.operationId, right.operationId)
}

function compareOperationId(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0
}

function strategyFor(candidates: Candidate[]): ToolRetrievalResult["strategy"] {
  if (!candidates.length) return "base_only"
  return candidates.some(candidate => candidate.reasonCode !== "goal_match") ? "lexical_workflow" : "lexical"
}

function queryText(query: ToolRetrievalQuery): string {
  return [
    bounded(query.currentGoal, 1200),
    bounded(query.routeName ?? "", 120),
    ...query.resourceContext.map(item => bounded(item, 120)),
    ...query.completedOperations.map(item => bounded(item, 120)),
    ...query.stableOutcomes.map(item => bounded(item, 120)),
    query.pendingState ?? "",
    ...query.stableErrorCodes.map(item => bounded(item, 160)),
  ].filter(Boolean).join("\n")
}

function bounded(input: string, maximumCharacters: number): string {
  return [...input].slice(0, maximumCharacters).join("")
}

function unique<T>(input: T[]): T[] {
  return [...new Set(input)]
}

function uniqueCandidates(input: Candidate[]): Candidate[] {
  const seen = new Set<string>()
  return input.filter(candidate => {
    if (seen.has(candidate.operationId)) return false
    seen.add(candidate.operationId)
    return true
  })
}

function appendCandidates(target: Candidate[], input: Candidate[], maximum: number): void {
  const seen = new Set(target.map(candidate => candidate.operationId))
  for (const candidate of input) {
    if (target.length >= maximum) return
    if (seen.has(candidate.operationId)) continue
    seen.add(candidate.operationId)
    target.push(candidate)
  }
}

function hasWriteSideEffect(operation: RetrievableTool): boolean {
  return operation.contract.sideEffect === "external-write"
    || operation.contract.sideEffect === "platform-write"
    || operation.contract.sideEffect === "destructive"
}

export type { ToolDocumentKind }
