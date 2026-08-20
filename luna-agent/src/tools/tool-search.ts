import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const searchToolsInput = z.object({
  query: z.string().trim().min(2).max(240),
  maxResults: z.number().int().min(1).max(12).default(8),
})

export type SearchToolsInput = z.infer<typeof searchToolsInput>

export const searchToolsTool: ModelToolDefinition = {
  operationId: "search_tools",
  description: "按任务目标检索 Luna DevOps 工具目录，并把匹配的真实工具加入当前 Run 的下一次模型步骤。当前可见工具可能是动态子集；仅当可见工具确实缺少所需能力、或需要在相似工具中复核边界时调用。query 应描述要完成的业务动作与资源，例如“为已部署服务创建公网 HTTPS 网关入口”，不要只写宽泛名词或猜测 operationId。检索命中后必须在同一 Run 继续调用返回的具体工具；不得用快捷选项询问用户是否要搜索，也不得把检索结果当成业务执行结果。同一目标最多检索一次，无命中时给出确定的能力缺口。",
  inputSchema: searchToolsJsonSchema(),
}

function searchToolsJsonSchema(): Record<string, unknown> {
  const schema = z.toJSONSchema(searchToolsInput, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  return schema
}
