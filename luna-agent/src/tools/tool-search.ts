import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const searchToolsInput = z.object({
  query: z.string().trim().max(240).default(""),
  page: z.number().int().min(1).max(100_000).default(1),
  pageSize: z.number().int().min(1).max(100).default(20),
}).strict()

export type SearchToolsInput = z.infer<typeof searchToolsInput>

export const searchToolsTool: ModelToolDefinition = {
  operationId: "search_tools",
  description: "浏览或检索 Luna DevOps 的完整工具摘要目录。query 留空时只分页浏览；query 非空时会返回摘要，并自动把最相关的少量候选加载为当前 Run 后续步骤可直接调用的平台工具。结果不执行平台操作，也不授予权限。只有需要精确参数语义、消歧或确认风险时才调用 get_tool_details。",
  inputSchema: jsonSchema(searchToolsInput),
}

function jsonSchema(schema: typeof searchToolsInput): Record<string, unknown> {
  const value = z.toJSONSchema(schema, { io: "input" }) as Record<string, unknown>
  delete value.$schema
  return value
}
