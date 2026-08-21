import { z } from "zod"
import { aiOptionIconNames } from "@luna-devops/ai-interaction-card-contract"
import type { AIOptionVisual } from "@luna-devops/ai-interaction-card-contract"
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

export const createOptionsTool: ModelToolDefinition = {
  operationId: "create_options",
  description: "当本轮回复确实存在清晰、可独立点选的后续操作时，展示 2～5 个简洁且符合当前上下文的快捷选项；任务正在等待表单提交、被批准阻塞、或没有下一步时不要调用。选项会在输入框上方以单行胶囊按钮呈现，因此 label 必须是可独立理解的纯文本短语，不写 emoji、句号、解释、编号或重复前缀；中文通常不超过 18 个字，其他语言不超过 32 个字符。description 不会在快捷栏展示，只在确有调试价值时提供。若每个候选都有清晰且一致的视觉语义，可以使用 visual 增加扫读性；整组选项必须全部使用相同 visual.type，无法为所有选项找到准确图形时必须全部省略，不得为了装饰强行添加。emoji 使用直观字符；icon 只能使用 schema 允许的项目图标名；img 必须来自当前可信工具结果中的 HTTPS 图片，不得编造链接。动作必须匹配最直接的用户意图：send_message 只用于已知候选的单击选择、自然语言澄清或继续分析；request_tool 用于提出参数已就绪的已注册操作；navigate 只用于读取、浏览或明确打开已知页面。需要用户填写、选择、切换或组合任何结构化操作参数时，必须改用 request_tool_input；需要从真实资源候选中单选时使用 request_resource_choice。不得生成要求用户编辑消息、替换占位符或随后继续输入参数的选项。不得把必要的资源选择变成页面跳转，也不得用无关跳转打断待完成的决定。选择快捷选项作为本轮后续交互时只调用一次本工具，不得再生成重复交互卡片。每个选项相互独立；导航默认可重复，消息和受控工具操作只能执行一次。不得声称操作已经执行，不得编造路由、ID 或操作，也不得用空泛建议代替对当前需求的直接回应。",
  inputSchema: optionsInputJsonSchema(),
}

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

function optionsInputJsonSchema(): Record<string, unknown> {
  const schema = z.toJSONSchema(createOptionsInput, { io: "input" }) as Record<string, unknown>
  delete schema.$schema
  return schema
}
