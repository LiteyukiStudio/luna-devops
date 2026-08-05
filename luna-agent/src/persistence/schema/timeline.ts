import { bigint, jsonb, text, timestamp, unique } from "drizzle-orm/pg-core"
import type { TimelineItem } from "../../domain.js"
import { aiSchema } from "./conversations.js"
import { runs } from "./runs.js"
import { turns } from "./turns.js"

export const items = aiSchema.table("items", {
  id: text("id").primaryKey(),
  runId: text("run_id").notNull().references(() => runs.id, { onDelete: "cascade" }),
  turnId: text("turn_id").notNull().references(() => turns.id, { onDelete: "cascade" }),
  timelineIndex: bigint("timeline_index", { mode: "number" }).notNull(),
  type: text("type").notNull().$type<TimelineItem["type"]>(),
  status: text("status").notNull().$type<TimelineItem["status"]>(),
  content: jsonb("content").notNull().$type<Record<string, unknown>>(),
  revision: bigint("revision", { mode: "number" }).notNull().default(1),
  createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
}, table => [
  unique("items_run_id_timeline_index_key").on(table.runId, table.timelineIndex),
])

export const runEvents = aiSchema.table("run_events", {
  id: text("id").primaryKey(),
  runId: text("run_id").notNull().references(() => runs.id, { onDelete: "cascade" }),
  eventSequence: bigint("event_sequence", { mode: "number" }).notNull(),
  type: text("type").notNull(),
  data: jsonb("data").notNull().$type<Record<string, unknown>>(),
  createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
}, table => [
  unique("run_events_run_id_event_sequence_key").on(table.runId, table.eventSequence),
])

export type TimelineItemRow = typeof items.$inferSelect
export type NewTimelineItemRow = typeof items.$inferInsert
export type RunEventRow = typeof runEvents.$inferSelect
export type NewRunEventRow = typeof runEvents.$inferInsert
