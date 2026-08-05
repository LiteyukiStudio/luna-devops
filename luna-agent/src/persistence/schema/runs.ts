import { bigint, index, integer, jsonb, text, timestamp, unique } from "drizzle-orm/pg-core"
import { sql } from "drizzle-orm"
import type { PromptVersion, RunStatus } from "../../domain.js"
import { aiSchema, conversations } from "./conversations.js"
import { turns } from "./turns.js"

export const runs = aiSchema.table("runs", {
  id: text("id").primaryKey(),
  ownerUserId: text("owner_user_id").notNull(),
  conversationId: text("conversation_id").notNull().references(() => conversations.id, { onDelete: "cascade" }),
  turnId: text("turn_id").notNull().references(() => turns.id, { onDelete: "cascade" }),
  runIndex: integer("run_index").notNull(),
  status: text("status").notNull().$type<RunStatus>(),
  rowVersion: integer("row_version").notNull().default(1),
  graphVersion: text("graph_version").notNull().$type<"assistant-v1">(),
  promptVersion: text("prompt_version").notNull().$type<PromptVersion>(),
  toolCatalogDigest: text("tool_catalog_digest").notNull(),
  pageContext: jsonb("page_context").notNull().$type<Record<string, unknown>>().default({}),
  runActorGrantCiphertext: text("run_actor_grant_ciphertext"),
  leaseOwner: text("lease_owner"),
  leaseExpiresAt: timestamp("lease_expires_at", { withTimezone: true, mode: "date" }),
  heartbeatAt: timestamp("heartbeat_at", { withTimezone: true, mode: "date" }),
  createdAt: timestamp("created_at", { withTimezone: true, mode: "date" }).notNull().defaultNow(),
  startedAt: timestamp("started_at", { withTimezone: true, mode: "date" }),
  completedAt: timestamp("completed_at", { withTimezone: true, mode: "date" }),
  errorCode: text("error_code"),
  clientInstanceId: text("client_instance_id"),
  traceContext: jsonb("trace_context").notNull().$type<Record<string, string>>().default({}),
  nextItemPosition: bigint("next_item_position", { mode: "number" }).notNull().default(0),
  nextEventSequence: bigint("next_event_sequence", { mode: "number" }).notNull().default(1),
}, table => [
  unique("runs_turn_id_run_index_key").on(table.turnId, table.runIndex),
  index("ai_runs_queue_idx").on(table.status, table.leaseExpiresAt).where(sql`${table.status} = 'queued'`),
])

export type RunRow = typeof runs.$inferSelect
export type NewRunRow = typeof runs.$inferInsert
