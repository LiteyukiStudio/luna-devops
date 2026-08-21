import type { LexicalTokenizer } from "./tokenizer.js"

export type RankedOperation = {
  operationId: string
  score: number
}

type IndexedDocument = {
  operationId: string
  fields: Map<string, IndexedField>
}

type IndexedField = { length: number, weight: number, termFrequency: Map<string, number> }

export type WeightedLexicalField = { text: string, weight: number }

/**
 * BM25F-style weighted lexical index. Each semantic field is normalized by its
 * own length before its contribution is combined, so an operation cannot win
 * merely because one string was repeated many times in a flattened document.
 */
export class BM25FIndex {
  private readonly documents: IndexedDocument[]
  private readonly documentFrequency = new Map<string, number>()
  private readonly averageFieldLengths = new Map<string, number>()

  constructor(
    documents: Array<{ operationId: string, fields: Record<string, WeightedLexicalField> }>,
    private readonly tokenizer: LexicalTokenizer,
    private readonly k1 = 1.2,
    private readonly b = 0.75,
  ) {
    const fieldLengthTotals = new Map<string, number>()
    const fieldDocumentCounts = new Map<string, number>()
    this.documents = documents.map((document) => {
      const fields = new Map<string, IndexedField>()
      const documentTerms = new Set<string>()
      for (const [name, field] of Object.entries(document.fields)) {
        if (!Number.isFinite(field.weight) || field.weight <= 0) continue
        const tokens = tokenizer.tokenize(field.text)
        if (!tokens.length) continue
        const termFrequency = frequencies(tokens)
        termFrequency.forEach((_count, term) => documentTerms.add(term))
        fields.set(name, { length: tokens.length, weight: field.weight, termFrequency })
        fieldLengthTotals.set(name, (fieldLengthTotals.get(name) ?? 0) + tokens.length)
        fieldDocumentCounts.set(name, (fieldDocumentCounts.get(name) ?? 0) + 1)
      }
      documentTerms.forEach(term => this.documentFrequency.set(term, (this.documentFrequency.get(term) ?? 0) + 1))
      return { operationId: document.operationId, fields }
    })
    fieldLengthTotals.forEach((total, name) => {
      this.averageFieldLengths.set(name, Math.max(1, total / (fieldDocumentCounts.get(name) ?? 1)))
    })
  }

  search(query: string, limit = Number.MAX_SAFE_INTEGER): RankedOperation[] {
    if (!this.documents.length) return []
    const queryTerms = new Set(this.tokenizer.tokenize(query))
    if (!queryTerms.size) return []
    return this.documents
      .map(document => ({ operationId: document.operationId, score: this.score(document, queryTerms) }))
      .filter(item => item.score > 0)
      .sort((left, right) => right.score - left.score || compareOperationId(left.operationId, right.operationId))
      .slice(0, Math.max(0, limit))
  }

  private score(document: IndexedDocument, queryTerms: Set<string>): number {
    let score = 0
    for (const term of queryTerms) {
      let weightedFrequency = 0
      for (const [name, field] of document.fields) {
        const frequency = field.termFrequency.get(term)
        if (!frequency) continue
        const averageLength = this.averageFieldLengths.get(name) ?? 1
        const normalized = frequency / (1 - this.b + this.b * field.length / averageLength)
        weightedFrequency += field.weight * normalized
      }
      if (!weightedFrequency) continue
      const containingDocuments = this.documentFrequency.get(term) ?? 0
      const inverseDocumentFrequency = Math.log(1 + ((this.documents.length - containingDocuments + 0.5) / (containingDocuments + 0.5)))
      score += inverseDocumentFrequency * (weightedFrequency * (this.k1 + 1)) / (weightedFrequency + this.k1)
    }
    return score
  }
}

export class BM25Index {
  private readonly documents: LegacyIndexedDocument[]
  private readonly documentFrequency = new Map<string, number>()
  private readonly averageDocumentLength: number

  constructor(
    documents: Array<{ operationId: string, tokens: string[] }>,
    private readonly tokenizer: LexicalTokenizer,
    private readonly k1 = 1.2,
    private readonly b = 0.75,
  ) {
    this.documents = documents.map(item => {
      const termFrequency = frequencies(item.tokens)
      for (const term of termFrequency.keys()) {
        this.documentFrequency.set(term, (this.documentFrequency.get(term) ?? 0) + 1)
      }
      return { operationId: item.operationId, length: item.tokens.length, termFrequency }
    })
    const totalLength = this.documents.reduce((total, item) => total + item.length, 0)
    this.averageDocumentLength = this.documents.length ? Math.max(1, totalLength / this.documents.length) : 1
  }

  search(query: string, limit = 30): RankedOperation[] {
    if (!this.documents.length) return []
    const queryTerms = new Set(this.tokenizer.tokenize(query))
    const ranked = this.documents.map(document => ({
      operationId: document.operationId,
      score: this.score(document, queryTerms),
    })).filter(item => item.score > 0)

    return ranked
      .sort((left, right) => right.score - left.score || compareOperationId(left.operationId, right.operationId))
      .slice(0, Math.max(0, limit))
  }

  private score(document: LegacyIndexedDocument, queryTerms: Set<string>): number {
    let score = 0
    for (const term of queryTerms) {
      const frequency = document.termFrequency.get(term)
      if (!frequency) continue
      const containingDocuments = this.documentFrequency.get(term) ?? 0
      const inverseDocumentFrequency = Math.log(1 + ((this.documents.length - containingDocuments + 0.5) / (containingDocuments + 0.5)))
      const lengthNormalization = frequency + this.k1 * (1 - this.b + this.b * document.length / this.averageDocumentLength)
      score += inverseDocumentFrequency * (frequency * (this.k1 + 1)) / lengthNormalization
    }
    return score
  }
}

type LegacyIndexedDocument = {
  operationId: string
  length: number
  termFrequency: Map<string, number>
}

function compareOperationId(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0
}

function frequencies(tokens: string[]): Map<string, number> {
  const output = new Map<string, number>()
  for (const token of tokens) output.set(token, (output.get(token) ?? 0) + 1)
  return output
}
