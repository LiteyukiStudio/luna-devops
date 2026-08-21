import { z } from "zod"
import { aiOptionIconNames } from "@luna-devops/ai-interaction-card-contract"
import type { AIOptionVisual } from "@luna-devops/ai-interaction-card-contract"
import { registeredRouteName, routeIdentifiers } from "./ui-route.js"

const optionAction = z.discriminatedUnion("type", [
  z.object({ type: z.literal("send_message"), message: z.string().trim().min(1).max(2000) }),
  z.object({ type: z.literal("navigate"), routeName: registeredRouteName, params: routeIdentifiers, query: routeIdentifiers }),
  z.object({
    type: z.literal("request_tool"),
    operationId: z.string().regex(/^[A-Za-z][A-Za-z0-9._-]{2,100}$/),
    arguments: z.record(z.string(), z.unknown()).default({}),
    message: z.string().trim().min(1).max(2000),
  }),
])
const optionVisual = z.discriminatedUnion("type", [
  z.object({ type: z.literal("emoji"), value: z.string().trim().min(1).max(16) }),
  z.object({ type: z.literal("icon"), value: z.enum(aiOptionIconNames) }),
  z.object({
    type: z.literal("img"),
    value: z.string().url().max(2048).refine(value => value.startsWith("https://"), "Option images must use HTTPS."),
  }),
])
export const createOptionsInput = z.object({
  title: z.string().trim().min(1).max(120),
  description: z.string().trim().max(300).optional(),
  options: z.array(
    z.object({
      id: z.string().regex(/^[a-zA-Z0-9_-]{1,40}$/),
      label: z.string().trim().min(1).max(40),
      description: z.string().trim().max(180).optional(),
      tone: z.enum(["default", "primary", "danger"]).default("default"),
      visual: optionVisual.optional(),
      repeatable: z.boolean().optional(),
      action: optionAction,
    }).superRefine((option, context) => {
      if (option.repeatable === true && option.action.type !== "navigate") {
        context.addIssue({
          code: "custom",
          message: "Only idempotent navigation options may be repeatable.",
          path: ["repeatable"],
        })
      }
    }).transform(option => ({
      ...option,
      repeatable: option.repeatable ?? option.action.type === "navigate",
    })),
  ).min(2).max(5),
}).superRefine((input, context) => {
  const ids = new Set<string>()
  input.options.forEach((option, index) => {
    if (ids.has(option.id)) {
      context.addIssue({
        code: "custom",
        message: "Option IDs must be unique.",
        path: ["options", index, "id"],
      })
    }
    ids.add(option.id)
  })
  const visualOptions = input.options.filter(option => option.visual !== undefined)
  if (visualOptions.length > 0 && visualOptions.length !== input.options.length) {
    input.options.forEach((option, index) => {
      if (!option.visual) {
        context.addIssue({
          code: "custom",
          message: "All options in a group must provide a visual, or none of them may provide one.",
          path: ["options", index, "visual"],
        })
      }
    })
  }
  const visualTypes = new Set(visualOptions.map(option => option.visual!.type))
  if (visualTypes.size > 1) {
    context.addIssue({
      code: "custom",
      message: "All option visuals in a group must use the same type.",
      path: ["options"],
    })
  }
})
export type CreateOptionsInput = z.infer<typeof createOptionsInput>
export type UIOptionAction =
  | { version: 1, id: string, repeatable: boolean, type: "navigate", label: string, description?: string, tone: "default" | "primary" | "danger", visual?: AIOptionVisual, payload: { routeName: z.infer<typeof registeredRouteName>, params: Record<string, string>, query: Record<string, string> } }
  | { version: 1, id: string, repeatable: boolean, type: "send_message", label: string, description?: string, tone: "default" | "primary" | "danger", visual?: AIOptionVisual, payload: { message: string } }
  | { version: 1, id: string, repeatable: boolean, type: "request_tool", label: string, description?: string, tone: "default" | "primary" | "danger", visual?: AIOptionVisual, payload: { operationId: string, arguments: Record<string, unknown>, message: string } }

export function optionUIActions(input: CreateOptionsInput): UIOptionAction[] {
  return input.options.map((option): UIOptionAction => {
    const base = {
      version: 1 as const,
      id: option.id,
      repeatable: option.repeatable,
      label: option.label,
      ...(option.description ? { description: option.description } : {}),
      ...(option.visual ? { visual: option.visual } : {}),
      tone: option.tone,
    }
    if (option.action.type === "navigate") return { ...base, type: "navigate", payload: { routeName: option.action.routeName, params: option.action.params, query: option.action.query } }
    if (option.action.type === "send_message") return { ...base, type: "send_message", payload: { message: option.action.message } }
    return { ...base, type: "request_tool", payload: { operationId: option.action.operationId, arguments: option.action.arguments, message: option.action.message } }
  })
}
