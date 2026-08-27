import type { Pool } from "pg"
import type { PayloadCipher } from "../payload-cipher.js"
import type { Repository } from "../persistence/repository.js"
import { redact } from "../redaction.js"
import type { ToolCallRecord, ToolCallStatus, ToolCallStore, ToolEvent } from "./orchestrator.js"

export class PostgresToolCallStore implements ToolCallStore {
  constructor(
    private readonly pool: Pool,
    private readonly repository: Repository,
    private readonly argumentsCipher: PayloadCipher,
  ) {}
  async insert(value: ToolCallRecord) {
    try {
      await this.pool.query(
		`insert into ai.tool_calls(id,run_id,operation_id,status,input_mode,arguments,arguments_ciphertext,arguments_hash,attempt,row_version,approval_decision,result,error_code)
		 values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
        [
          value.id,
          value.runId,
          value.operationId,
          value.status,
          value.inputMode ?? "model",
          JSON.stringify(redact(value.arguments)),
          this.argumentsCipher.encrypt(JSON.stringify(value.arguments)),
          value.argumentsHash,
          value.attempt,
          value.rowVersion,
		  value.approvalDecision ?? null,
			value.result === undefined ? null : JSON.stringify(value.result),
          value.errorCode ?? null,
        ],
      )
    }
    catch (error) {
      throw toolPersistenceError(error)
    }
  }
  async get(id: string) {
    const row = (await this.pool.query<DbToolCall>(`select * from ai.tool_calls where id=$1`, [id])).rows[0]
    return row ? this.map(row) : undefined
  }
  async update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>) {
    const row = (await this.pool.query<DbToolCall>(
		 `update ai.tool_calls set status=coalesce($3,status),row_version=coalesce($4,row_version),
		 approval_decision=coalesce($5,approval_decision),
		 result=coalesce($6,result),error_code=coalesce($7,error_code),updated_at=now()
		 where id=$1 and status=$2 returning *`,
		[id, expected, patch.status ?? null, patch.rowVersion ?? null,
		 patch.approvalDecision ?? null, patch.result === undefined ? null : JSON.stringify(patch.result), patch.errorCode ?? null],
    )).rows[0]
    if (!row) throw new Error("ai.tool_call_state_conflict")
    return this.map(row)
  }
  async emit(event: ToolEvent) {
    const call = await this.get(event.toolCallId)
    if (!call) throw new Error("ai.tool_call_not_found")
    const input = await this.pool.query<{ turn_id: string }>(`select turn_id from ai.runs where id=$1`, [call.runId])
    const turnId = input.rows[0]?.turn_id
    if (!turnId) return
    const itemId = `${call.id}:item`
    const content = {
        toolCallId: call.id, operationId: call.operationId, status: call.status,
        arguments: redact(call.arguments), result: call.result, errorCode: call.errorCode,
		approvalDecision: call.approvalDecision,
        ...(typeof event.data.durationMs === "number" && Number.isFinite(event.data.durationMs)
          ? { durationMs: Math.max(0, Math.round(event.data.durationMs)) }
          : {}),
        ...(typeof event.data.traceId === "string" && /^(?!0{32}$)[a-f0-9]{32}$/i.test(event.data.traceId)
          ? { traceId: event.data.traceId }
          : {}),
    }
    const publicType = publicToolEventType(event.type)
    const eventData = { itemId, toolCallId: call.id, ...event.data }
    if (event.type === "tool.started") {
      await this.repository.appendItemWithEvent({ id: itemId, runId: call.runId, turnId, type: "tool_call", status: "streaming", content }, publicType, eventData)
    }
    else if (isToolTerminalEvent(event.type)) {
      await this.repository.completeToolItemWithEvent(itemId, toolItemStatus(event.type), content, {
        id: `${call.id}:result`, runId: call.runId, turnId, type: "tool_result",
        status: event.type === "tool_call.failed" ? "failed" : "completed",
        content: { relatedItemId: itemId, result: call.result, errorCode: call.errorCode },
      }, publicType, eventData)
    }
    else {
      await this.repository.updateItemWithEvent(itemId, toolItemStatus(event.type), content, publicType, eventData)
    }
  }
  private map(row: DbToolCall): ToolCallRecord {
    if (!row.arguments_ciphertext)
      throw new Error("ai.tool_arguments_key_unavailable")
    const argumentsValue = JSON.parse(this.argumentsCipher.decrypt(row.arguments_ciphertext)) as Record<string, unknown>
    return {
      id: row.id, runId: row.run_id, operationId: row.operation_id, status: row.status,
      arguments: argumentsValue, argumentsHash: row.arguments_hash, attempt: row.attempt, rowVersion: row.row_version,
      ...(row.input_mode === "direct" || row.input_mode === "model" ? { inputMode: row.input_mode } : {}),
      ...(row.approval_decision ? { approvalDecision: row.approval_decision } : {}),
      ...(row.result !== null ? { result: row.result } : {}),
      ...(row.error_code ? { errorCode: row.error_code } : {}),
    }
  }
}

function isToolTerminalEvent(type: string) {
  return type === "tool_call.succeeded" || type === "tool_call.failed" || type === "tool_call.rejected"
}

function toolItemStatus(type: string): "streaming" | "completed" | "failed" {
  if (type === "tool_call.failed") return "failed"
  return type === "tool_call.succeeded" || type === "tool_call.rejected" ? "completed" : "streaming"
}

function publicToolEventType(type: string) {
  if (type === "tool_call.running") return "tool.progress"
  if (type === "tool_call.succeeded") return "tool.completed"
  if (type === "tool_call.failed") return "tool.failed"
  if (type === "tool_call.rejected") return "tool.rejected"
  return type
}

type DbToolCall = {
  id: string; run_id: string; operation_id: string; status: ToolCallStatus; input_mode: string; arguments: Record<string, unknown>;
  arguments_ciphertext: string | null;
  arguments_hash: string; attempt: number; row_version: number;
	approval_decision: "approve" | null;
	result: unknown; error_code: string | null
}

function toolPersistenceError(error: unknown): Error {
  const postgresCode = error && typeof error === "object" && "code" in error
    ? String(error.code)
    : ""
  return new Error(postgresCode === "42703" ? "ai.database_schema_mismatch" : "ai.tool_persistence_failed")
}
