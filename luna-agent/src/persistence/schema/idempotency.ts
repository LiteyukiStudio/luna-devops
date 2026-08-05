import { primaryKey, text, timestamp } from "drizzle-orm/pg-core"
import { aiSchema } from "./conversations.js"
import { runs } from "./runs.js"
import { turns } from "./turns.js"

export const idempotencyKeys = aiSchema.table("idempotency_keys", {
  ownerUserId: text("owner_user_id").notNull(),
  idempotencyKey: text("idempotency_key").notNull(),
  requestHash: text("request_hash").notNull(),
  turnId: text("turn_id").notNull().references(() => turns.id, { onDelete: "cascade" }),
  runId: text("run_id").notNull().references(() => runs.id, { onDelete: "cascade" }),
  createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
}, table => [
  primaryKey({ columns: [table.ownerUserId, table.idempotencyKey] }),
])

export type IdempotencyKeyRow = typeof idempotencyKeys.$inferSelect
export type NewIdempotencyKeyRow = typeof idempotencyKeys.$inferInsert
