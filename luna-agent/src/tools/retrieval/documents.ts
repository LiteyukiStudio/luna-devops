import type { AgentToolContract } from "../contracts.js"
import type { LexicalTokenizer } from "./tokenizer.js"

export type ToolDocumentKind = "intent" | "parameters" | "workflow"

export type RetrievableTool = {
  operationId: string
  category: string
  description?: string | undefined
  searchHints?: string[] | undefined
  inputSchema: {
    properties: Record<string, Record<string, unknown>>
  }
  contract: AgentToolContract
}

export type ToolRetrievalDocument = {
  operationId: string
  kind: ToolDocumentKind
  text: string
  tokens: string[]
}

export type ToolRetrievalDocuments = {
  vectors: Record<ToolDocumentKind, ToolRetrievalDocument>
  lexical: ToolRetrievalDocument
}

export function buildToolRetrievalDocuments(
  operation: RetrievableTool,
  tokenizer: LexicalTokenizer,
): ToolRetrievalDocuments {
  const contract = operation.contract
  const inputNames = Object.keys(operation.inputSchema.properties)
  const verifier = contract.verification.mode === "response" ? [] : [contract.verification.operationId]

  const intent = document(operation.operationId, "intent", tokenizer, [
    operation.category,
    ...contract.resourceTypes,
    contract.action,
    ...contract.intents,
    ...contract.useWhen,
    operation.description ?? "",
    ...(operation.searchHints ?? []),
  ])
  const parameters = document(operation.operationId, "parameters", tokenizer, [
    ...inputNames,
    ...contract.parameterSummary,
    ...contract.commonErrorCodes,
  ])
  const workflow = document(operation.operationId, "workflow", tokenizer, [
    ...contract.prerequisites,
    ...contract.predecessors,
    ...contract.followups,
    ...verifier,
    ...contract.successEvidence,
  ])

  return {
    vectors: { intent, parameters, workflow },
    lexical: document(operation.operationId, "intent", tokenizer, [
      operation.operationId,
      operation.category,
      ...contract.resourceTypes,
      contract.action,
      ...contract.intents,
      ...contract.useWhen,
      ...inputNames,
      ...contract.parameterSummary,
      ...contract.commonErrorCodes,
      ...contract.predecessors,
      ...contract.followups,
      ...verifier,
      ...contract.successEvidence,
    ]),
  }
}

function document(
  operationId: string,
  kind: ToolDocumentKind,
  tokenizer: LexicalTokenizer,
  parts: string[],
): ToolRetrievalDocument {
  const text = parts.map(part => part.trim()).filter(Boolean).join("\n")
  return { operationId, kind, text, tokens: tokenizer.tokenize(text) }
}
