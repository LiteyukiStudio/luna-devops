import { index, integer, jsonb, text, timestamp } from "drizzle-orm/pg-core"
import type { ConversationSummaryContent } from "../../domain.js"
import { aiSchema, conversations } from "./conversations.js"

export const conversationSummaries = aiSchema.table("conversation_summaries", {
  conversationId: text("conversation_id").primaryKey().references(() => conversations.id, { onDelete: "cascade" }),
  coveredThroughTurnIndex: integer("covered_through_turn_index").notNull(),
  compressionVersion: integer("compression_version").notNull().$type<1>(),
  sourceTurnCount: integer("source_turn_count").notNull(),
  content: jsonb("content").notNull().$type<ConversationSummaryContent>(),
  createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
}, table => [
  index("ai_conversation_summaries_updated_idx").on(table.updatedAt.desc()),
])

export type ConversationSummaryRow = typeof conversationSummaries.$inferSelect
export type NewConversationSummaryRow = typeof conversationSummaries.$inferInsert
