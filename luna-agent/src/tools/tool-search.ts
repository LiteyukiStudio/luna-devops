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
  description: "浏览或检索 Luna DevOps 的完整工具摘要目录。query 可以留空以按页浏览；也可以描述资源、动作、工具名或 operationId。结果只包含名称、用途、标签、别名和是否高危，不包含参数 Schema，也不会执行平台操作。确定候选后必须调用 get_tool_details 加载一到八个精确 operationId。",
  inputSchema: jsonSchema(searchToolsInput),
}

function jsonSchema(schema: typeof searchToolsInput): Record<string, unknown> {
  const value = z.toJSONSchema(schema, { io: "input" }) as Record<string, unknown>
  delete value.$schema
  return value
}
