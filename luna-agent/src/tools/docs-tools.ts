import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const searchDocsInput = z.object({
  query: z.string().trim().min(1).max(240),
  locale: z.enum(["zh", "en"]).optional(),
  limit: z.number().int().min(1).max(8).default(5),
}).strict()

export const listDocsInput = z.object({
  locale: z.enum(["zh", "en"]),
  path: z.string().trim().max(240).default(""),
  depth: z.number().int().min(1).max(3).default(1),
}).strict()

export const readDocInput = z.object({
  docId: z.string().trim().min(1).max(240),
  sectionId: z.string().trim().max(240).optional(),
}).strict()

export type SearchDocsInput = z.infer<typeof searchDocsInput>
export type ReadDocInput = z.infer<typeof readDocInput>
export type ListDocsInput = z.infer<typeof listDocsInput>

export const searchDocsTool: ModelToolDefinition = {
  operationId: "search_docs",
  description: "检索随当前平台版本发布的 Luna DevOps 公开文档，回答概念、操作步骤、配置语法、模板变量、取值范围、限制和排障方法。locale 按用户语言选 zh 或 en。返回章节摘要、docId、sectionId 和可选文档链接；需要完整内容时用 read_doc 读取。文档只说明产品行为，不代表任何资源的实时状态，也不执行或授权平台操作。",
  inputSchema: jsonSchema(searchDocsInput),
}

export const listDocsTool: ModelToolDefinition = {
  operationId: "list_docs",
  description: "像 ls 一样按文档站目录树浏览公开文档：只返回目录名称、文档标题和一句话描述，不含正文。path 为目录路径（空字符串表示语言根目录，例如 use/workflow），depth 为展开层数。关键词检索 search_docs 可能因措辞不同漏掉内容时，用它逐级定位相关文档，再用 read_doc 读取。",
  inputSchema: jsonSchema(listDocsInput),
}

export const readDocTool: ModelToolDefinition = {
  operationId: "read_doc",
  description: "按 search_docs 或 list_docs 返回的 docId（可选 sectionId）读取公开文档全文或单个章节，内容有长度上限并返回 truncated 标志和章节目录。用于在回答前核对配置语法、变量列表、默认值与限制的原文。",
  inputSchema: jsonSchema(readDocInput),
}

function jsonSchema(schema: z.ZodType): Record<string, unknown> {
  const value = z.toJSONSchema(schema, { io: "input" }) as Record<string, unknown>
  delete value.$schema
  return value
}
