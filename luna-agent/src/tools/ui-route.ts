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
  description: "Immediately switch the signed-in user's current browser view to a registered Luna DevOps route without reloading the page. Use only when the user explicitly asks to open, go to, or switch to a known page, or when an immediate route change is necessary and unambiguous. Do not use this for optional suggestions; use create_options instead. Never invent route names or identifiers.",
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
