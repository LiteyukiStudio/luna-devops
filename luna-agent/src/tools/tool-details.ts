import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const getToolDetailsInput = z.object({
  operationIds: z.array(z.string().regex(/^[A-Za-z][A-Za-z0-9._-]{2,100}$/)).min(1).max(8),
}).strict()

export type GetToolDetailsInput = z.infer<typeof getToolDetailsInput>

export const getToolDetailsTool: ModelToolDefinition = {
  operationId: "get_tool_details",
  description: "按精确 operationId 加载一到八个 Luna DevOps 工具的紧凑语义详情，用于参数含义、相似能力消歧、风险、前置条件与成功证据确认。被选工具会在当前 Run 后续所有模型步骤持续可用；调用 Schema 由运行时单独注入，详情不暴露执行路由，也不执行或授权平台操作。",
  inputSchema: jsonSchema(getToolDetailsInput),
}

function jsonSchema(schema: typeof getToolDetailsInput): Record<string, unknown> {
  const value = z.toJSONSchema(schema, { io: "input" }) as Record<string, unknown>
  delete value.$schema
  return value
}
