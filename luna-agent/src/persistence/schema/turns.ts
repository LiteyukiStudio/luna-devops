import { integer, text, timestamp, unique } from "drizzle-orm/pg-core"
import type { RunStatus } from "../../domain.js"
import { aiSchema } from "./conversations.js"
import { conversations } from "./conversations.js"

export const turns = aiSchema.table("turns", {
  id: text("id").primaryKey(),
  conversationId: text("conversation_id").notNull().references(() => conversations.id, { onDelete: "cascade" }),
  turnIndex: integer("turn_index").notNull(),
  status: text("status").notNull().$type<RunStatus>(),
  input: text("input").notNull(),
  selectedRunId: text("selected_run_id").notNull(),
  modelId: text("model_id"),
  createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
}, table => [
  unique("turns_conversation_id_turn_index_key").on(table.conversationId, table.turnIndex),
])

export type TurnRow = typeof turns.$inferSelect
export type NewTurnRow = typeof turns.$inferInsert
