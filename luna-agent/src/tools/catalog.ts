import { createHash } from "node:crypto"
import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"
import { validateToolArguments } from "./argument-validator.js"
import {
  type ToolCatalogDetails,
  type ToolCatalogPage,
  type ToolCatalogSummary,
  type ToolSemanticDetails,
  toolAliasesSchema,
  toolLocalizedListSchema,
  toolLocalizedTextSchema,
  toolRouteParameterSchema,
} from "./contracts.js"
import { BM25FIndex, type WeightedLexicalField } from "./retrieval/bm25.js"
import { UnicodeLexicalTokenizer } from "./retrieval/tokenizer.js"

const inputSchema = z.object({
  type: z.literal("object"),
  properties: z.record(z.string(), z.record(z.string(), z.unknown())).default({}),
  required: z.array(z.string()).default([]),
  additionalProperties: z.literal(false),
}).passthrough()

const outputSchema = z.record(z.string(), z.unknown())

const operationSchema = z.object({
  operationId: z.string().regex(/^[A-Za-z][A-Za-z0-9._-]{2,100}$/),
  name: z.string().trim().min(1).max(160).optional(),
  summary: z.string().trim().min(1).max(1000).optional(),
  category: z.string().trim().min(1).max(120),
  tags: z.array(z.string().trim().min(1).max(120)).max(30).default([]),
  aliases: toolAliasesSchema.default({ zh: [], en: [] }),
  purpose: toolLocalizedTextSchema.default({ zh: "", en: "" }),
  avoidWhen: toolLocalizedTextSchema.default({ zh: "", en: "" }),
  preconditions: toolLocalizedListSchema.default({ zh: [], en: [] }),
  successEvidence: toolLocalizedTextSchema.default({ zh: "", en: "" }),
  requiresApproval: z.boolean().default(false),
  idempotent: z.boolean(),
  method: z.enum(["GET", "POST", "PUT", "PATCH", "DELETE"]),
  path: z.string().startsWith("/api/v1/"),
  requiredScopes: z.array(z.string().trim().min(1).max(120)).max(20),
  inputSchema,
  outputSchema: outputSchema.default({}),
  sensitivePaths: z.array(z.string().trim().min(1).max(240)).max(100).default([]),
  parameters: z.array(toolRouteParameterSchema).max(100).default([]),
  requestBody: z.boolean().default(false),
  requestRequired: z.boolean().default(false),
  requestType: z.string().trim().max(120).default(""),
}).transform(value => ({
  ...value,
  name: value.name ?? value.operationId,
  summary: value.summary ?? value.name ?? value.operationId,
  tags: value.tags.length ? unique(value.tags) : [value.category],
  aliases: {
    zh: unique(value.aliases.zh),
    en: unique(value.aliases.en),
  },
  purpose: localizedText(value.purpose, conservativePurpose(value.operationId, value.summary ?? value.name ?? value.operationId)),
  avoidWhen: localizedText(value.avoidWhen),
  successEvidence: localizedText(value.successEvidence),
}))

export type ToolOperation = z.infer<typeof operationSchema>

export type ToolSearchInput = {
  query?: string
  page?: number
  pageSize?: number
}

const defaultPageSize = 20
const maximumPageSize = 100

export class ToolCatalog {
  private readonly operations: Map<string, ToolOperation>
  private readonly orderedOperations: ToolOperation[]
  private readonly bm25: BM25FIndex
  readonly digest: string

  private constructor(values: ToolOperation[]) {
    this.operations = new Map(values.map(value => [value.operationId, value]))
    if (this.operations.size !== values.length) throw new Error("ai.tool_catalog_duplicate_operation")
    this.orderedOperations = [...values].sort(compareOperations)
    const tokenizer = new UnicodeLexicalTokenizer()
    this.bm25 = new BM25FIndex(this.orderedOperations.map(operation => ({
      operationId: operation.operationId,
      fields: searchFields(operation),
    })), tokenizer)
    this.digest = `sha256:${createHash("sha256").update(JSON.stringify(this.orderedOperations)).digest("hex")}`
  }

  static load(input: unknown): ToolCatalog {
    return new ToolCatalog(z.array(operationSchema).min(1).parse(input))
  }

  get(operationId: string): ToolOperation {
    const value = this.operations.get(operationId)
    if (!value) throw new Error("ai.tool_not_available")
    return value
  }

  all(): ToolOperation[] {
    return [...this.orderedOperations]
  }

  search(input: ToolSearchInput = {}): ToolCatalogPage {
    const query = bounded(input.query?.trim() ?? "", 240)
    const page = boundedInteger(input.page, 1, 100_000, 1)
    const pageSize = boundedInteger(input.pageSize, 1, maximumPageSize, defaultPageSize)
    const matching = query
      ? this.bm25.search(query, this.orderedOperations.length)
          .sort((left, right) => {
            const leftOperation = this.operations.get(left.operationId)
            const rightOperation = this.operations.get(right.operationId)
            const bonus = (operation: ToolOperation | undefined) => operation ? exactIntentBonus(operation, query) : 0
            return bonus(rightOperation) - bonus(leftOperation)
              || right.score - left.score
              || compareOperationsById(left.operationId, right.operationId)
          })
          .map(result => this.operations.get(result.operationId))
          .filter((operation): operation is ToolOperation => operation !== undefined)
      : this.orderedOperations
    const offset = (page - 1) * pageSize
    return {
      query,
      items: matching.slice(offset, offset + pageSize).map(toSummary),
      page,
      pageSize,
      total: matching.length,
      totalPages: matching.length ? Math.ceil(matching.length / pageSize) : 0,
    }
  }

  getDetails(operationIds: string[]): ToolCatalogDetails<ToolOperation> {
    const requested = unique(operationIds)
    const items = requested.flatMap((operationId) => {
      const operation = this.operations.get(operationId)
      return operation ? [operation] : []
    })
    const found = new Set(items.map(item => item.operationId))
    return {
      items,
      missingOperationIds: requested.filter(operationId => !found.has(operationId)),
    }
  }

  semanticDetails(operationIds: string[]): ToolCatalogDetails<ToolSemanticDetails> {
    const details = this.getDetails(operationIds)
    return { items: details.items.map(toSemanticDetails), missingOperationIds: details.missingOperationIds }
  }

  modelTools(operationIds: string[]): ModelToolDefinition[] {
    return this.getDetails(operationIds).items.map(operation => ({
      operationId: operation.operationId,
      description: modelDescription(operation),
      inputSchema: operation.inputSchema,
    }))
  }
}

function toSummary(operation: ToolOperation): ToolCatalogSummary {
  return {
    operationId: operation.operationId,
    name: operation.name,
    summary: operation.summary,
    category: operation.category,
    tags: operation.tags,
    aliases: operation.aliases,
    purpose: operation.purpose,
    avoidWhen: operation.avoidWhen,
    preconditions: operation.preconditions,
    successEvidence: operation.successEvidence,
    requiresApproval: operation.requiresApproval,
  }
}

function exactIntentBonus(operation: ToolOperation, query: string): number {
  const normalizedQuery = normalizeSearchPhrase(query)
  if (!normalizedQuery) return 0
  if (normalizeSearchPhrase(operation.operationId) === normalizedQuery) return 3
  if ([...operation.aliases.zh, ...operation.aliases.en].some(alias => normalizeSearchPhrase(alias) === normalizedQuery)) return 2
  if (normalizeSearchPhrase(operation.name) === normalizedQuery || normalizeSearchPhrase(operation.purpose.zh) === normalizedQuery || normalizeSearchPhrase(operation.purpose.en) === normalizedQuery) return 1
  return 0
}

function normalizeSearchPhrase(value: string): string {
  return value.normalize("NFKC").toLocaleLowerCase("zh-CN").replaceAll(/\s+/g, " ").trim()
}

function modelDescription(operation: ToolOperation): string {
  const purpose = operation.purpose.zh || operation.summary
  const avoid = operation.avoidWhen.zh ? `不适用场景：${operation.avoidWhen.zh}。` : ""
  const preconditions = operation.preconditions.zh.length ? `前置条件：${operation.preconditions.zh.join("；")}。` : ""
  const evidence = operation.successEvidence.zh ? `成功证据：${operation.successEvidence.zh}。` : ""
  const approval = operation.requiresApproval ? "这是高风险操作，执行前需要用户批准。" : ""
  return `${operation.name}。用途：${purpose}。${avoid}${preconditions}${evidence}${approval}`
}

function searchFields(operation: ToolOperation): Record<string, WeightedLexicalField> {
  const identifiers = splitIdentifier(operation.operationId).join(" ")
  const parameters = operation.parameters.flatMap(parameter => [parameter.inputName, parameter.wireName])
  const input = schemaTerms(operation.inputSchema)
  const output = schemaTerms(operation.outputSchema)
  return {
    operationId: { text: operation.operationId, weight: 10 },
    actionResource: { text: identifiers, weight: 7 },
    name: { text: operation.name, weight: 6 },
    aliasesZh: { text: operation.aliases.zh.join("\n"), weight: 9 },
    aliasesEn: { text: operation.aliases.en.join("\n"), weight: 7 },
    purpose: { text: `${operation.purpose.zh}\n${operation.purpose.en}`, weight: 6 },
    summary: { text: operation.summary, weight: 4 },
    category: { text: operation.category, weight: 3 },
    tags: { text: operation.tags.join("\n"), weight: 3 },
    parameters: { text: parameters.join("\n"), weight: 4 },
    inputSchema: { text: input.join("\n"), weight: 2.5 },
    outputSchema: { text: output.join("\n"), weight: 2.5 },
    avoidWhen: { text: `${operation.avoidWhen.zh}\n${operation.avoidWhen.en}`, weight: 1.5 },
    preconditions: { text: [...operation.preconditions.zh, ...operation.preconditions.en].join("\n"), weight: 2 },
    successEvidence: { text: `${operation.successEvidence.zh}\n${operation.successEvidence.en}`, weight: 3 },
  }
}

function toSemanticDetails(operation: ToolOperation): ToolSemanticDetails {
  const properties = objectProperties(operation.inputSchema)
  const required = new Set(Array.isArray(operation.inputSchema.required) ? operation.inputSchema.required.filter((item): item is string => typeof item === "string") : [])
  return {
    ...toSummary(operation),
    idempotent: operation.idempotent,
    requiredScopes: operation.requiredScopes,
    majorParameters: Object.entries(properties).slice(0, 20).map(([name, schema]) => ({
      name,
      required: required.has(name),
      ...(typeof schema.description === "string" && schema.description.trim() ? { description: schema.description.trim() } : {}),
    })),
    outputFields: Object.entries(objectProperties(operation.outputSchema)).slice(0, 20).map(([name, schema]) => ({
      name,
      ...(typeof schema.description === "string" && schema.description.trim() ? { description: schema.description.trim() } : {}),
    })),
    errorBehavior: operation.requiresApproval
      ? "参数会经过严格 Schema 校验；权限不足或审批未通过时不会执行，平台错误以稳定错误码返回。"
      : "参数会经过严格 Schema 校验；权限不足或平台失败时以稳定错误码返回。",
  }
}

function schemaTerms(schema: Record<string, unknown>, prefix = "", depth = 0): string[] {
  if (depth > 8) return []
  const output: string[] = []
  for (const [name, value] of Object.entries(objectProperties(schema))) {
    const path = prefix ? `${prefix}.${name}` : name
    output.push(path, name)
    if (typeof value.description === "string") output.push(value.description)
    output.push(...schemaTerms(value, path, depth + 1))
  }
  if (schema.items && typeof schema.items === "object" && !Array.isArray(schema.items))
    output.push(...schemaTerms(schema.items as Record<string, unknown>, `${prefix}[]`, depth + 1))
  return output
}

function objectProperties(schema: Record<string, unknown>): Record<string, Record<string, unknown>> {
  if (!schema.properties || typeof schema.properties !== "object" || Array.isArray(schema.properties)) return {}
  return Object.fromEntries(Object.entries(schema.properties as Record<string, unknown>)
    .filter((entry): entry is [string, Record<string, unknown>] => Boolean(entry[1]) && typeof entry[1] === "object" && !Array.isArray(entry[1])))
}

function splitIdentifier(value: string): string[] {
  return value.replace(/([a-z0-9])([A-Z])/g, "$1 $2").split(/[._\-/\s]+/).map(part => part.toLowerCase()).filter(Boolean)
}

function localizedText(value: { zh: string, en: string }, fallback = "") {
  return { zh: value.zh || fallback, en: value.en }
}

function conservativePurpose(operationId: string, summary: string): string {
  return `用于调用 ${operationId} 对应的平台能力。${summary ? `平台摘要：${summary}` : ""}`
}

function compareOperations(left: ToolOperation, right: ToolOperation): number {
  return compareOperationsById(left.operationId, right.operationId)
}

function compareOperationsById(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0
}

function boundedInteger(value: number | undefined, minimum: number, maximum: number, fallback: number): number {
  return Number.isSafeInteger(value) ? Math.max(minimum, Math.min(maximum, value!)) : fallback
}

function bounded(value: string, maximumCharacters: number): string {
  return [...value].slice(0, maximumCharacters).join("")
}

function unique<T>(input: T[]): T[] {
  return [...new Set(input)]
}

export function validateArguments(schema: ToolOperation["inputSchema"], input: unknown): Record<string, unknown> {
  return validateToolArguments(schema, input)
}
