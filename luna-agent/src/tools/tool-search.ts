import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const searchToolsInput = z.object({
  query: z.string().trim().min(2).max(240),
  maxResults: z.number().int().min(1).max(12).default(8),
})

export type SearchToolsInput = z.infer<typeof searchToolsInput>

export const searchToolsTool: ModelToolDefinition = {
  operationId: "search_tools",
  description: "按任务目标检索当前未加载的 Luna DevOps 工具，并把最相关的真实工具加入本轮后续模型步骤。当前工具集无法完成下一步、你准备声称平台缺少某项能力、或存在多个相似工具而无法确定时，必须先调用本工具。query 应描述要完成的业务动作与资源，例如“为已部署服务创建公网 HTTPS 网关入口”，不要只写一个宽泛名词，也不要猜测 operationId。检索只发现能力，不执行平台操作、不代替鉴权、批准、MFA、参数收集或完成验收；取得结果后必须继续调用返回的具体工具。当前工具已经足够时不要调用。",
  inputSchema: searchToolsJsonSchema(),
}

function searchToolsJsonSchema(): Record<string, unknown> {
  const schema = z.toJSONSchema(searchToolsInput, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  return schema
}
