import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const routeIdentifiers = z.record(z.string(), z.string().regex(/^[\w.:-]{1,160}$/)).default({})
export const registeredRouteName = z.enum([
  "dashboard", "projects", "project.workspace", "application.detail", "events",
  "code-repositories", "registries", "clusters", "app-templates", "billing",
  "settings.account", "settings.auth-providers", "settings.notifications",
  "settings.operations", "settings.site", "settings.users",
])
export const navigateToRouteInput = z.object({
  routeName: registeredRouteName,
  params: routeIdentifiers,
  query: routeIdentifiers,
})
export type NavigateToRouteInput = z.infer<typeof navigateToRouteInput>

export const navigateToRouteTool: ModelToolDefinition = {
  operationId: "navigate_to_route",
  description: "在不刷新页面的情况下，将当前登录用户的浏览器视图立即切换到已注册的 Luna DevOps 路由。只要用户的主要意图唯一对应另一个已注册专用页面就必须使用；用户不必逐字说出“打开”或“跳转”。目标必须唯一，资源页参数必须来自页面上下文、用户输入或可信工具结果；无资源 ID 的全局页面可直接切换。跳转只同步当前视图，必须继续执行任务所需的业务工具，不能代替结构化输入、批准或验收。可选建议使用 create_options；不得为纯后台查询、无关建议、尚未确定的候选、正在填写的表单或待批准操作制造意外跳转。绝不能编造路由名或资源标识符。",
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

export function automaticRouteUIAction(input: NavigateToRouteInput) {
  return {
    version: 1 as const,
    type: "navigate" as const,
    activation: "automatic" as const,
    repeatable: false,
    payload: {
      routeName: input.routeName,
      params: input.params,
      query: input.query,
    },
  }
}
