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
  description: "在不刷新页面的情况下，将当前登录用户的浏览器视图立即切换到已注册的 Luna DevOps 路由。只有用户明确要求打开、前往或切换到已知页面，或者立即跳转确有必要且没有歧义时才可使用。可选建议应使用 create_options，不得调用本工具。绝不能编造路由名或资源标识符。",
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
