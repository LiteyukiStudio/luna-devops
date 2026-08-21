import { z } from "zod"

export const toolAliasesSchema = z.object({
  zh: z.array(z.string().trim().min(1).max(120)).default([]),
  en: z.array(z.string().trim().min(1).max(120)).default([]),
}).strict()

export type ToolAliases = z.infer<typeof toolAliasesSchema>

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
