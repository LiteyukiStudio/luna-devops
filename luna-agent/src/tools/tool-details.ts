import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const getToolDetailsInput = z.object({
  operationIds: z.array(z.string().regex(/^[A-Za-z][A-Za-z0-9._-]{2,100}$/)).min(1).max(8),
}).strict()

export type GetToolDetailsInput = z.infer<typeof getToolDetailsInput>

export const getToolDetailsTool: ModelToolDefinition = {
  operationId: "get_tool_details",
  description: "按精确 operationId 加载一到八个 Luna DevOps 工具的完整调用详情，包括输入 Schema、返回 Schema、权限范围、路由参数和高危审批标记。只加载当前任务真正选择的工具；详情本身不执行平台操作。",
  inputSchema: jsonSchema(getToolDetailsInput),
}

function jsonSchema(schema: typeof getToolDetailsInput): Record<string, unknown> {
  const value = z.toJSONSchema(schema, { io: "input" }) as Record<string, unknown>
  delete value.$schema
  return value
}
