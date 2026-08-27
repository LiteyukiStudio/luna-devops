import { z } from "zod"
import { aiInternalRouteNames } from "@luna-devops/ai-interaction-card-contract"
import type { ModelToolDefinition } from "../provider/provider.js"

export const routeIdentifiers = z.record(z.string(), z.string().regex(/^[\w.:-]{1,160}$/)).default({})
export const registeredRouteName = z.enum(aiInternalRouteNames)
export const navigateToRouteInput = z.object({
  routeName: registeredRouteName,
  params: routeIdentifiers,
  query: routeIdentifiers,
})
export type NavigateToRouteInput = z.infer<typeof navigateToRouteInput>

export const navigateToRouteTool: ModelToolDefinition = {
  operationId: "navigate_to_route",
  description: "在会话时间线中生成一个指向已注册 Luna DevOps 内部路由的可点击按钮，不会自动切换当前页面。仅当用户需要查看另一个专用页面时使用；目标必须唯一，资源页参数必须来自页面上下文、用户输入或可信工具结果。导航记录不能代替结构化输入、批准、业务操作或验收。候选未确定时可使用 request_choice；绝不能编造路由名、资源标识符或跨源地址。",
  inputSchema: {
    type: "object",
    additionalProperties: false,
    required: ["routeName"],
    properties: {
      routeName: { type: "string", enum: registeredRouteName.options },
      params: { type: "object", additionalProperties: { type: "string" } },
      query: { type: "object", additionalProperties: { type: "string" } },
    },
  },
}

export function routeUIAction(input: NavigateToRouteInput) {
  return {
    version: 1 as const,
    type: "navigate" as const,
    activation: "manual" as const,
    repeatable: false,
    payload: {
      routeName: input.routeName,
      params: input.params,
      query: input.query,
    },
  }
}
