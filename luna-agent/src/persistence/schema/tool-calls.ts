import { index, integer, jsonb, text, timestamp } from "drizzle-orm/pg-core"
import { aiSchema } from "./conversations.js"
import { runs } from "./runs.js"

export const toolCalls = aiSchema.table("tool_calls", {
  id: text("id").primaryKey(),
  runId: text("run_id").notNull().references(() => runs.id, { onDelete: "cascade" }),
  operationId: text("operation_id").notNull(),
  status: text("status").notNull(),
  inputMode: text("input_mode").notNull().default("model"),
  arguments: jsonb("arguments").notNull().$type<Record<string, unknown>>(),
  argumentsCiphertext: text("arguments_ciphertext"),
  argumentsHash: text("arguments_hash").notNull(),
  attempt: integer("attempt").notNull().default(1),
  rowVersion: integer("row_version").notNull().default(1),
  approvalDecision: text("approval_decision").$type<"approve">(),
  result: jsonb("result").$type<unknown>(),
  errorCode: text("error_code"),
  createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
}, table => [
  index("ai_tool_calls_run_created_idx").on(table.runId, table.createdAt),
])

export type ToolCallRow = typeof toolCalls.$inferSelect
export type NewToolCallRow = typeof toolCalls.$inferInsert
