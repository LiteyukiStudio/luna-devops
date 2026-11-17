import { bigint, index, integer, jsonb, numeric, text, timestamp, unique } from "drizzle-orm/pg-core"
import type { ModelCallOperation } from "../repository.js"
import { aiSchema } from "./conversations.js"

export const modelUsages = aiSchema.table("model_usages", {
  id: text("id").primaryKey(),
  runId: text("run_id").notNull(),
  ownerUserId: text("owner_user_id").notNull(),
  operation: text("operation").notNull().$type<ModelCallOperation>(),
  attempt: integer("attempt").notNull(),
  status: text("status").notNull().$type<"reported">(),
  settlementStatus: text("settlement_status").notNull().$type<"pending" | "settled">(),
  modelId: text("model_id").notNull(),
  modelName: text("model_name").notNull(),
  maxContextTokensSnapshot: bigint("max_context_tokens_snapshot", { mode: "number" }).notNull(),
  maxOutputTokensSnapshot: bigint("max_output_tokens_snapshot", { mode: "number" }).notNull().default(65_536),
  inputCreditsPerMillion: numeric("input_credits_per_million", { precision: 24, scale: 8 }).notNull().default("0"),
  outputCreditsPerMillion: numeric("output_credits_per_million", { precision: 24, scale: 8 }).notNull().default("0"),
  cachedInputCreditsPerMillion: numeric("cached_input_credits_per_million", { precision: 24, scale: 8 }).notNull().default("0"),
  promptTokens: bigint("prompt_tokens", { mode: "number" }).notNull(),
  completionTokens: bigint("completion_tokens", { mode: "number" }).notNull(),
  totalTokens: bigint("total_tokens", { mode: "number" }).notNull(),
  cachedPromptTokens: bigint("cached_prompt_tokens", { mode: "number" }),
  cacheWritePromptTokens: bigint("cache_write_prompt_tokens", { mode: "number" }),
  reasoningCompletionTokens: bigint("reasoning_completion_tokens", { mode: "number" }),
  providerRequestId: text("provider_request_id"),
  responseId: text("response_id"),
  responseModel: text("response_model"),
  finishReason: text("finish_reason"),
  callType: text("call_type").notNull().$type<"stream" | "complete">(),
  officialDetails: jsonb("official_details").notNull().$type<Record<string, unknown>>().default({}),
  occurredAt: timestamp("occurred_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
  settledAt: timestamp("settled_at", { withTimezone: true, mode: "date" }),
}, table => [
  unique("model_usages_run_id_operation_attempt_key").on(table.runId, table.operation, table.attempt),
  index("ai_model_usages_run_idx").on(table.runId, table.operation, table.occurredAt.desc()),
  index("ai_model_usages_settlement_idx").on(table.settlementStatus, table.occurredAt),
  index("ai_model_usages_owner_settlement_idx").on(table.ownerUserId, table.settlementStatus),
])

export type ModelUsageRow = typeof modelUsages.$inferSelect
export type NewModelUsageRow = typeof modelUsages.$inferInsert
