import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const renameConversationInput = z.object({
  title: z.string().trim().min(1).max(60),
}).strict()

export const renameConversationTool: ModelToolDefinition = {
  operationId: "rename_conversation",
  description: "使用用户当前语言为会话设置简洁标题。titleSource 为 default 时在首轮调用；titleSource 为 assistant 且原有标题不再符合主要话题时可以再次调用；titleSource 为 user 时绝不能调用。",
  inputSchema: {
    type: "object",
    properties: {
      title: {
        type: "string",
        minLength: 1,
        maxLength: 60,
        description: "简洁的话题标题，不包含引号、Markdown 或句末标点。",
      },
    },
    required: ["title"],
    additionalProperties: false,
  },
}
