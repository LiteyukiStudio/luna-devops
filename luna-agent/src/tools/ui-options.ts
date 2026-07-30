import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"
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
export const createOptionsInput = z.object({
  title: z.string().trim().min(1).max(120),
  description: z.string().trim().max(300).optional(),
  options: z.array(
    z.object({
      id: z.string().regex(/^[a-zA-Z0-9_-]{1,40}$/),
      label: z.string().trim().min(1).max(80),
      description: z.string().trim().max(180).optional(),
      tone: z.enum(["default", "primary", "danger"]).default("default"),
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
})
export type CreateOptionsInput = z.infer<typeof createOptionsInput>
export type UIOptionAction =
  | { version: 1, id: string, repeatable: boolean, type: "navigate", label: string, description?: string, tone: "default" | "primary" | "danger", payload: { routeName: z.infer<typeof registeredRouteName>, params: Record<string, string>, query: Record<string, string> } }
  | { version: 1, id: string, repeatable: boolean, type: "send_message", label: string, description?: string, tone: "default" | "primary" | "danger", payload: { message: string } }
  | { version: 1, id: string, repeatable: boolean, type: "request_tool", label: string, description?: string, tone: "default" | "primary" | "danger", payload: { operationId: string, arguments: Record<string, unknown>, message: string } }

export const createOptionsTool: ModelToolDefinition = {
  operationId: "create_options",
  description: "展示 2～5 个简洁且符合当前上下文的后续操作。动作必须匹配最直接的用户意图：send_message 用于回答待选择问题或收集缺失参数；request_tool 用于提出参数已就绪的已注册操作；navigate 只用于读取、浏览或明确打开已知页面。不得把必要的资源选择变成页面跳转，也不得用无关跳转打断待完成的决定。每个正常完成的轮次必须以且仅以一次 create_options 调用结束。每个选项相互独立；导航默认可重复，消息和受控工具操作只能执行一次。不得声称操作已经执行，不得编造路由、ID 或操作，也不得用空泛建议代替对当前需求的直接回应。",
  inputSchema: {
    type: "object",
    additionalProperties: false,
    required: ["title", "options"],
    properties: {
      title: { type: "string", maxLength: 120 },
      description: { type: "string", maxLength: 300 },
      options: {
        type: "array",
        minItems: 2,
        maxItems: 5,
        items: {
          type: "object",
          additionalProperties: false,
          required: ["id", "label", "action"],
          properties: {
            id: { type: "string", maxLength: 40 },
            label: { type: "string", maxLength: 80 },
            description: { type: "string", maxLength: 180 },
            tone: { type: "string", enum: ["default", "primary", "danger"] },
            repeatable: { type: "boolean", description: "仅导航动作可选。navigate 默认为 true，消息或工具请求默认为 false。" },
            action: {
              oneOf: [
                { type: "object", additionalProperties: false, required: ["type", "message"], properties: { type: { const: "send_message" }, message: { type: "string", maxLength: 2000 } } },
                { type: "object", additionalProperties: false, required: ["type", "routeName"], properties: { type: { const: "navigate" }, routeName: { type: "string", enum: registeredRouteName.options }, params: { type: "object", additionalProperties: { type: "string" } }, query: { type: "object", additionalProperties: { type: "string" } } } },
                { type: "object", additionalProperties: false, required: ["type", "operationId", "message"], properties: { type: { const: "request_tool" }, operationId: { type: "string" }, arguments: { type: "object" }, message: { type: "string", maxLength: 2000 } } },
              ],
            },
          },
        },
      },
    },
  },
}

export function optionUIActions(input: CreateOptionsInput): UIOptionAction[] {
  return input.options.map((option): UIOptionAction => {
    const base = {
      version: 1 as const,
      id: option.id,
      repeatable: option.repeatable,
      label: option.label,
      ...(option.description ? { description: option.description } : {}),
      tone: option.tone,
    }
    if (option.action.type === "navigate") return { ...base, type: "navigate", payload: { routeName: option.action.routeName, params: option.action.params, query: option.action.query } }
    if (option.action.type === "send_message") return { ...base, type: "send_message", payload: { message: option.action.message } }
    return { ...base, type: "request_tool", payload: { operationId: option.action.operationId, arguments: option.action.arguments, message: option.action.message } }
  })
}
