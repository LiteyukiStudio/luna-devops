import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

const listToolsInput = z.object({
  mode: z.literal("list"),
  category: z.string().trim().min(1).max(80).optional(),
  page: z.number().int().min(1).max(1000).default(1),
  pageSize: z.number().int().min(1).max(200).default(100),
}).strict()

const toolDetailsInput = z.object({
  mode: z.literal("details"),
  operationIds: z.array(z.string().regex(/^[A-Za-z][A-Za-z0-9._-]{2,100}$/)).min(1).max(8),
}).strict()

export const browseToolsInput = z.discriminatedUnion("mode", [listToolsInput, toolDetailsInput])

export const browseToolsTool: ModelToolDefinition = {
  operationId: "browse_tools",
  description: "确定性浏览 Luna DevOps 已审计并准入的工具目录。当前可见工具缺少所需能力时，先用 list 只读取 operationId、分类、风险和一句话用途；识别候选后用 details 按精确 operationId 加载完整说明与参数 Schema，并在同一 Run 继续调用具体工具。该目录只展示能力，不授予权限、不执行平台操作，也不能作为业务完成证据。无法从轻量列表确定候选时，才使用 search_tools 做语义检索。",
  inputSchema: browseToolsJsonSchema(),
}

function browseToolsJsonSchema(): Record<string, unknown> {
  const schema = z.toJSONSchema(browseToolsInput, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  return schema
}
