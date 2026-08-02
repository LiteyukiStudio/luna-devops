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
    await this.pool.query(
      `insert into ai.tool_calls(id,run_id,operation_id,status,arguments,arguments_ciphertext,arguments_hash,attempt,row_version,approval_expires_at,mfa_purpose)
       values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
      [
        value.id,
        value.runId,
        value.operationId,
        value.status,
        JSON.stringify(redact(value.arguments)),
        this.argumentsCipher.encrypt(JSON.stringify(value.arguments)),
        value.argumentsHash,
        value.attempt,
        value.rowVersion,
        value.approvalExpiresAt ? new Date(value.approvalExpiresAt) : null,
        value.mfaPurpose ?? null,
      ],
    )
  }
  async get(id: string) {
    const row = (await this.pool.query<DbToolCall>(`select * from ai.tool_calls where id=$1`, [id])).rows[0]
    return row ? this.map(row) : undefined
  }
  async update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>) {
    const row = (await this.pool.query<DbToolCall>(
      `update ai.tool_calls set status=coalesce($3,status),row_version=coalesce($4,row_version),
       approval_expires_at=coalesce($5,approval_expires_at),mfa_purpose=coalesce($6,mfa_purpose),
       result=coalesce($7,result),error_code=coalesce($8,error_code),updated_at=now()
       where id=$1 and status=$2 returning *`,
      [id, expected, patch.status ?? null, patch.rowVersion ?? null, patch.approvalExpiresAt ? new Date(patch.approvalExpiresAt) : null, patch.mfaPurpose ?? null, patch.result ? JSON.stringify(patch.result) : null, patch.errorCode ?? null],
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
        argumentsHash: call.argumentsHash, expectedVersion: call.rowVersion, mfaPurpose: call.mfaPurpose,
        ...(typeof event.data.durationMs === "number" && Number.isFinite(event.data.durationMs)
          ? { durationMs: Math.max(0, Math.round(event.data.durationMs)) }
          : {}),
        ...(typeof event.data.traceId === "string" && /^(?!0{32}$)[a-f0-9]{32}$/i.test(event.data.traceId)
          ? { traceId: event.data.traceId }
          : {}),
    }
    const publicType = publicToolEventType(event.type)
    const eventData = { itemId, toolCallId: call.id, ...event.data }
    await (event.type === "tool.started"
      ? this.repository.appendItemWithEvent({ id: itemId, runId: call.runId, turnId, type: "tool_call", status: "streaming", content }, publicType, eventData)
      : this.repository.updateItemWithEvent(itemId, toolItemStatus(event.type), content, publicType, eventData))
    if (isToolTerminalEvent(event.type)) {
      await this.repository.appendItem({
        id: `${call.id}:result`, runId: call.runId, turnId, type: "tool_result",
        status: event.type === "tool_call.failed" ? "failed" : "completed",
        content: { relatedItemId: itemId, result: call.result, errorCode: call.errorCode },
      })
    }
  }
  async listAwaitingApproval(runId: string) {
    const rows = (await this.pool.query<DbToolCall>(
      `select * from ai.tool_calls where run_id=$1 and status='awaiting_approval' order by created_at`,
      [runId],
    )).rows
    return rows.map(row => this.map(row))
  }

  private map(row: DbToolCall): ToolCallRecord {
    if (!row.arguments_ciphertext)
      throw new Error("ai.tool_arguments_key_unavailable")
    const argumentsValue = JSON.parse(this.argumentsCipher.decrypt(row.arguments_ciphertext)) as Record<string, unknown>
    return {
      id: row.id, runId: row.run_id, operationId: row.operation_id, status: row.status,
      arguments: argumentsValue, argumentsHash: row.arguments_hash, attempt: row.attempt, rowVersion: row.row_version,
      ...(row.approval_expires_at ? { approvalExpiresAt: row.approval_expires_at.getTime() } : {}),
      ...(row.mfa_purpose ? { mfaPurpose: row.mfa_purpose } : {}),
      ...(row.result !== null ? { result: row.result } : {}),
      ...(row.error_code ? { errorCode: row.error_code } : {}),
    }
  }
}

function isToolTerminalEvent(type: string) {
  return type === "tool_call.succeeded" || type === "tool_call.failed"
}

function toolItemStatus(type: string): "streaming" | "completed" | "failed" {
  if (type === "tool_call.failed") return "failed"
  return type === "tool_call.succeeded" || type === "approval.resolved" ? "completed" : "streaming"
}

function publicToolEventType(type: string) {
  if (type === "tool_call.running") return "tool.progress"
  if (type === "tool_call.succeeded") return "tool.completed"
  if (type === "tool_call.failed") return "tool.failed"
  if (type === "tool_call.awaiting_mfa") return "mfa.required"
  return type
}

type DbToolCall = {
  id: string; run_id: string; operation_id: string; status: ToolCallStatus; arguments: Record<string, unknown>;
  arguments_ciphertext: string | null;
  arguments_hash: string; attempt: number; row_version: number; approval_expires_at: Date | null;
  mfa_purpose: string | null; result: unknown; error_code: string | null
}
