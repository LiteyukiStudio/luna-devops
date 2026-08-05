import { index, integer, jsonb, text, timestamp, unique } from "drizzle-orm/pg-core"
import { sql } from "drizzle-orm"
import type { UIActionStatus } from "../../domain.js"
import { aiSchema } from "./conversations.js"
import { runs } from "./runs.js"

export const uiActions = aiSchema.table("ui_actions", {
  id: text("id").primaryKey(),
  runId: text("run_id").notNull().references(() => runs.id, { onDelete: "cascade" }),
  toolCallId: text("tool_call_id").notNull(),
  clientInstanceId: text("client_instance_id").notNull(),
  action: jsonb("action").notNull().$type<Record<string, unknown>>(),
  status: text("status").notNull().$type<UIActionStatus>().default("pending"),
  attempts: integer("attempts").notNull().default(1),
  expiresAt: timestamp("expires_at", { withTimezone: true, mode: "date" }).notNull(),
  acknowledgedAt: timestamp("acknowledged_at", { withTimezone: true, mode: "date" }),
  actualPath: text("actual_path"),
  errorCode: text("error_code"),
  createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
}, table => [
  unique("ui_actions_tool_call_id_key").on(table.toolCallId),
  index("ai_ui_actions_pending_client_idx").on(table.clientInstanceId, table.createdAt).where(sql`${table.status} = 'pending'`),
])

export type UIActionRow = typeof uiActions.$inferSelect
export type NewUIActionRow = typeof uiActions.$inferInsert
