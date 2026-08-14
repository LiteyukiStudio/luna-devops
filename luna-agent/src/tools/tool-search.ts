import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const searchToolsInput = z.object({
  query: z.string().trim().min(2).max(240),
  maxResults: z.number().int().min(1).max(12).default(8),
})

export type SearchToolsInput = z.infer<typeof searchToolsInput>

export const searchToolsTool: ModelToolDefinition = {
  operationId: "search_tools",
  description: "按任务目标检索 Luna DevOps 工具目录，并把匹配的真实工具加入本轮后续模型步骤。平台工具已随每次模型请求全量下发，通常无需调用本工具；仅在确认当前工具列表确实缺少某项能力、或需要在众多相似工具中复核存在性时使用。query 应描述要完成的业务动作与资源，例如“为已部署服务创建公网 HTTPS 网关入口”，不要只写一个宽泛名词，也不要猜测 operationId。检索只发现能力，不执行平台操作、不代替鉴权、批准、MFA、参数收集或完成验收；取得结果后必须继续调用返回的具体工具。当前工具已经足够时不要调用。",
  inputSchema: searchToolsJsonSchema(),
}

function searchToolsJsonSchema(): Record<string, unknown> {
  const schema = z.toJSONSchema(searchToolsInput, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  return schema
}
