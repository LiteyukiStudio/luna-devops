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
  description: "Present 2-5 concise, context-specific predictions of what the user is most likely to do next. Every normally completed turn must end with exactly one create_options call. Each option is independent: selecting one never disables its siblings. Registered route navigation is repeatable by default; message and controlled-tool actions are non-idempotent and may run only once. Never claim the option already executed, never invent route names or operation IDs, and never use generic suggestions instead of answering a direct factual question.",
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
            repeatable: { type: "boolean", description: "Optional for navigation only. Defaults to true for navigate and false for message or tool requests." },
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

export function fallbackOptionsInput(pageContext: Record<string, unknown>): CreateOptionsInput {
  const locale = typeof pageContext.locale === "string" ? pageContext.locale : ""
  const chinese = locale.toLowerCase().startsWith("zh")
  const projectId = identifier(pageContext.projectId)
  const applicationId = identifier(pageContext.applicationId)
  const route = typeof pageContext.routeName === "string" ? pageContext.routeName : "unknown"
  const followUp = {
    id: "continue-analysis",
    label: chinese ? "继续深入分析" : "Continue the analysis",
    action: {
      type: "send_message" as const,
      message: chinese
        ? "请基于刚才的结果继续深入分析，并给出最值得优先处理的一步。"
        : "Continue from the previous result and identify the single highest-priority next step.",
    },
  }
  const riskReview = {
    id: "review-risks",
    label: chinese ? "查看风险和检查项" : "Review risks and checks",
    action: {
      type: "send_message" as const,
      message: chinese
        ? "请总结当前结果中的风险、待确认信息和检查清单。"
        : "Summarize the risks, unknowns, and verification checklist from the current result.",
    },
  }

  const options: Array<z.input<typeof createOptionsInput>["options"][number]> = [followUp, riskReview]
  if (route === "application.detail" && projectId && applicationId) {
    options.splice(0, 0, {
      id: "open-builds",
      label: chinese ? "查看应用构建" : "Open application builds",
      action: { type: "navigate", routeName: "application.detail", params: { projectId, applicationId }, query: { tab: "builds" } },
    })
  } else if (route === "project.workspace" && projectId) {
    options.splice(0, 0, {
      id: "open-applications",
      label: chinese ? "查看项目应用" : "Open project applications",
      action: { type: "navigate", routeName: "project.workspace", params: { projectId }, query: { tab: "apps" } },
    })
  } else {
    options.splice(0, 0, {
      id: route === "projects" ? "open-dashboard" : "open-projects",
      label: route === "projects"
        ? (chinese ? "返回平台看板" : "Open dashboard")
        : (chinese ? "查看项目空间" : "Open projects"),
      action: { type: "navigate", routeName: route === "projects" ? "dashboard" : "projects", params: {}, query: {} },
    })
  }
  return createOptionsInput.parse({
    title: chinese ? "你接下来可能想做" : "You may want to",
    options,
  })
}

function identifier(value: unknown) {
  return typeof value === "string" && /^[\w.:-]{1,160}$/.test(value) ? value : undefined
}
