import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const renameConversationInput = z.object({
  title: z.string().trim().min(1).max(60),
}).strict()

export const renameConversationTool: ModelToolDefinition = {
  operationId: "rename_conversation",
  description: "使用用户当前语言为会话设置简洁标题。此工具只在当前会话允许助手改名时提供；主要话题明显变化时调用。首轮默认标题由执行层兜底，不要在话题未变化时反复调用。",
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
