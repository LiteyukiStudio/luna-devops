import { bigint, integer, jsonb, text, timestamp } from "drizzle-orm/pg-core"
import type { RunCheckpointPhase, RunCheckpointState } from "../../domain.js"
import { aiSchema } from "./conversations.js"
import { runs } from "./runs.js"

export const runCheckpoints = aiSchema.table("run_checkpoints", {
  runId: text("run_id").primaryKey().references(() => runs.id, { onDelete: "cascade" }),
  revision: bigint("revision", { mode: "number" }).notNull().default(1),
  phase: text("phase").notNull().$type<RunCheckpointPhase>(),
  modelStep: integer("model_step").notNull().default(0),
  toolCallCount: integer("tool_call_count").notNull().default(0),
  recoveries: integer("recoveries").notNull().default(0),
  state: jsonb("state").notNull().$type<RunCheckpointState>().default({}),
  createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
  updatedAt: timestamp("updated_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
})

export type RunCheckpointRow = typeof runCheckpoints.$inferSelect
export type NewRunCheckpointRow = typeof runCheckpoints.$inferInsert
