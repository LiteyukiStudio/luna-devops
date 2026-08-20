import type { ToolDocumentKind, ToolRetrievalDocument } from "./documents.js"
import type { RankedOperation } from "./bm25.js"

export interface EmbeddingProvider {
  identity(): { provider: string, model: string, dimensions: number }
  embedDocuments(input: string[], signal?: AbortSignal): Promise<number[][]>
  embedQuery(input: string, signal?: AbortSignal): Promise<number[]>
}

export type DenseRecall = Record<ToolDocumentKind, RankedOperation[]>

type VectorDocument = {
  operationId: string
  kind: ToolDocumentKind
  vector: number[]
}

export class InMemoryToolVectorIndex {
  private constructor(
    private readonly providerIdentity: ReturnType<EmbeddingProvider["identity"]>,
    private readonly documents: VectorDocument[],
  ) {}

  static async build(
    provider: EmbeddingProvider,
    documents: ToolRetrievalDocument[],
    signal?: AbortSignal,
  ): Promise<InMemoryToolVectorIndex> {
    const identity = validateIdentity(provider.identity())
    const vectors = await provider.embedDocuments(documents.map(item => item.text), signal)
    if (vectors.length !== documents.length) throw new Error("ai.tool_embedding_document_count_invalid")

    return new InMemoryToolVectorIndex(identity, documents.map((document, index) => ({
      operationId: document.operationId,
      kind: document.kind,
      vector: normalizeVector(vectors[index] ?? [], identity.dimensions),
    })))
  }

  async search(
    query: string,
    provider: EmbeddingProvider,
    limits: Record<ToolDocumentKind, number> = { intent: 30, parameters: 30, workflow: 20 },
    signal?: AbortSignal,
  ): Promise<DenseRecall> {
    const identity = validateIdentity(provider.identity())
    if (!sameIdentity(identity, this.providerIdentity)) throw new Error("ai.tool_embedding_provider_changed")
    const queryVector = normalizeVector(await provider.embedQuery(query, signal), identity.dimensions)

    return {
      intent: this.rank(queryVector, "intent", limits.intent),
      parameters: this.rank(queryVector, "parameters", limits.parameters),
      workflow: this.rank(queryVector, "workflow", limits.workflow),
    }
  }

  private rank(queryVector: number[], kind: ToolDocumentKind, limit: number): RankedOperation[] {
    return this.documents
      .filter(document => document.kind === kind)
      .map(document => ({ operationId: document.operationId, score: dot(queryVector, document.vector) }))
      .filter(item => item.score > 0)
      .sort((left, right) => right.score - left.score || compareOperationId(left.operationId, right.operationId))
      .slice(0, Math.max(0, limit))
  }
}

function compareOperationId(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0
}

function validateIdentity(identity: ReturnType<EmbeddingProvider["identity"]>): ReturnType<EmbeddingProvider["identity"]> {
  if (!identity.provider.trim() || !identity.model.trim() || !Number.isSafeInteger(identity.dimensions) || identity.dimensions < 1) {
    throw new Error("ai.tool_embedding_identity_invalid")
  }
  return identity
}

function sameIdentity(
  left: ReturnType<EmbeddingProvider["identity"]>,
  right: ReturnType<EmbeddingProvider["identity"]>,
): boolean {
  return left.provider === right.provider && left.model === right.model && left.dimensions === right.dimensions
}

function normalizeVector(vector: number[], dimensions: number): number[] {
  if (vector.length !== dimensions || vector.some(value => !Number.isFinite(value))) {
    throw new Error("ai.tool_embedding_vector_invalid")
  }
  const magnitude = Math.sqrt(vector.reduce((total, value) => total + value * value, 0))
  if (!Number.isFinite(magnitude) || magnitude === 0) throw new Error("ai.tool_embedding_vector_invalid")
  return vector.map(value => value / magnitude)
}

function dot(left: number[], right: number[]): number {
  let result = 0
  for (let index = 0; index < left.length; index += 1) result += (left[index] ?? 0) * (right[index] ?? 0)
  return result
}
