import { index, pgSchema, text, timestamp } from "drizzle-orm/pg-core"

export const aiSchema = pgSchema("ai")

export const conversations = aiSchema.table("conversations", {
  id: text("id").primaryKey(),
  ownerUserId: text("owner_user_id").notNull(),
  projectId: text("project_id"),
  modelId: text("model_id"),
  title: text("title").notNull(),
  // 数据库当前使用 text + check constraint，类型约束在 TypeScript 边界完成
  titleSource: text("title_source").notNull().$type<"default" | "assistant" | "user">().default("default"),
  status: text("status").notNull().$type<"active">().default("active"),
  createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
}, table => [
  index("ai_conversations_owner_updated_idx").on(table.ownerUserId, table.updatedAt.desc()),
])

export type ConversationRow = typeof conversations.$inferSelect
export type NewConversationRow = typeof conversations.$inferInsert
