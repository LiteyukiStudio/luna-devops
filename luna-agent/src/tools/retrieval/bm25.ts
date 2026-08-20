import type { LexicalTokenizer } from "./tokenizer.js"

export type RankedOperation = {
  operationId: string
  score: number
}

type IndexedDocument = {
  operationId: string
  length: number
  termFrequency: Map<string, number>
}

export class BM25Index {
  private readonly documents: IndexedDocument[]
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

  private score(document: IndexedDocument, queryTerms: Set<string>): number {
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

function compareOperationId(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0
}

function frequencies(tokens: string[]): Map<string, number> {
  const output = new Map<string, number>()
  for (const token of tokens) output.set(token, (output.get(token) ?? 0) + 1)
  return output
}
