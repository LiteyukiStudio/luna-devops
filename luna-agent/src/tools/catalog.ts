import { createHash } from "node:crypto"
import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"
import { validateToolArguments } from "./argument-validator.js"
import {
  type ToolCatalogDetails,
  type ToolCatalogPage,
  type ToolCatalogSummary,
  toolAliasesSchema,
  toolRouteParameterSchema,
} from "./contracts.js"
import { BM25Index } from "./retrieval/bm25.js"
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
  private readonly bm25: BM25Index
  readonly digest: string

  private constructor(values: ToolOperation[]) {
    this.operations = new Map(values.map(value => [value.operationId, value]))
    if (this.operations.size !== values.length) throw new Error("ai.tool_catalog_duplicate_operation")
    this.orderedOperations = [...values].sort(compareOperations)
    const tokenizer = new UnicodeLexicalTokenizer()
    this.bm25 = new BM25Index(this.orderedOperations.map(operation => ({
      operationId: operation.operationId,
      tokens: tokenizer.tokenize(searchDocument(operation)),
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
    requiresApproval: operation.requiresApproval,
  }
}

function modelDescription(operation: ToolOperation): string {
  const approval = operation.requiresApproval ? "这是高危操作，执行前需要用户批准。" : ""
  return `${operation.name}。${operation.summary}${approval}`
}

function searchDocument(operation: ToolOperation): string {
  return [
    operation.operationId,
    operation.operationId,
    operation.name,
    operation.name,
    operation.summary,
    operation.category,
    ...operation.tags,
    ...operation.aliases.zh,
    ...operation.aliases.zh,
    ...operation.aliases.en,
    ...operation.aliases.en,
  ].join("\n")
}

function compareOperations(left: ToolOperation, right: ToolOperation): number {
  return left.operationId < right.operationId ? -1 : left.operationId > right.operationId ? 1 : 0
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
