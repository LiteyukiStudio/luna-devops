import { z } from "zod"

export const toolAliasesSchema = z.object({
  zh: z.array(z.string().trim().min(1).max(120)),
  en: z.array(z.string().trim().min(1).max(120)),
}).strict()

export type ToolAliases = z.infer<typeof toolAliasesSchema>

export const toolLocalizedTextSchema = z.object({
  zh: z.string().trim().max(1000),
  en: z.string().trim().max(1000),
}).strict()

export const toolLocalizedListSchema = z.object({
  zh: z.array(z.string().trim().min(1).max(500)).max(20),
  en: z.array(z.string().trim().min(1).max(500)).max(20),
}).strict()

export type ToolLocalizedText = z.infer<typeof toolLocalizedTextSchema>
export type ToolLocalizedList = z.infer<typeof toolLocalizedListSchema>

export const toolRouteParameterSchema = z.object({
  inputName: z.string().trim().min(1).max(120),
  wireName: z.string().trim().min(1).max(120),
  in: z.enum(["path", "query", "header"]),
  required: z.boolean(),
}).strict()

export type ToolRouteParameter = z.infer<typeof toolRouteParameterSchema>

export type ToolCatalogSummary = {
  operationId: string
  name: string
  summary: string
  category: string
  tags: string[]
  aliases: ToolAliases
  purpose: ToolLocalizedText
  avoidWhen: ToolLocalizedText
  preconditions: ToolLocalizedList
  successEvidence: ToolLocalizedText
  requiresApproval: boolean
}

export type ToolCatalogPage = {
  query: string
  items: ToolCatalogSummary[]
  page: number
  pageSize: number
  total: number
  totalPages: number
}

export type ToolCatalogDetails<T> = {
  items: T[]
  missingOperationIds: string[]
}

export type ToolSemanticDetails = ToolCatalogSummary & {
  idempotent: boolean
  requiredScopes: string[]
  majorParameters: Array<{ name: string, required: boolean, description?: string }>
  outputFields: Array<{ name: string, description?: string }>
  errorBehavior: string
}

export type ToolArgumentIssue = {
  path: string
  code: string
  allowedValues?: unknown[]
  remediation?: string
}

export type ToolArgumentsInvalid = {
  code: "ai.tool_arguments_invalid"
  retryable: boolean
  issues: ToolArgumentIssue[]
}
