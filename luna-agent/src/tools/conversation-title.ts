import { z } from "zod"
import type { ModelToolDefinition } from "../provider/provider.js"

export const renameConversationInput = z.object({
  title: z.string().trim().min(1).max(60),
}).strict()

export const renameConversationTool: ModelToolDefinition = {
  operationId: "rename_conversation",
  description: "Rename the current conversation to a concise title in the user's language. Use this on the first turn when titleSource is default, or later when an assistant-generated title no longer matches the main topic. Never call it when titleSource is user.",
  inputSchema: {
    type: "object",
    properties: {
      title: {
        type: "string",
        minLength: 1,
        maxLength: 60,
        description: "A concise topic title without quotes, markdown, or trailing punctuation.",
      },
    },
    required: ["title"],
    additionalProperties: false,
  },
}
