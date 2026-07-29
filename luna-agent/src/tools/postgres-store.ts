import type { Pool } from "pg"
import type { Repository } from "../persistence/repository.js"
import type { ToolCallRecord, ToolCallStatus, ToolCallStore, ToolEvent } from "./orchestrator.js"

export class PostgresToolCallStore implements ToolCallStore {
  constructor(private readonly pool: Pool, private readonly repository: Repository) {}
  async insert(value: ToolCallRecord) {
    await this.pool.query(
      `insert into ai.tool_calls(id,run_id,operation_id,status,arguments,arguments_hash,attempt,row_version,approval_expires_at,mfa_purpose)
       values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
      [value.id, value.runId, value.operationId, value.status, JSON.stringify(value.arguments), value.argumentsHash, value.attempt, value.rowVersion, value.approvalExpiresAt ? new Date(value.approvalExpiresAt) : null, value.mfaPurpose ?? null],
    )
  }
  async get(id: string) {
    const row = (await this.pool.query<DbToolCall>(`select * from ai.tool_calls where id=$1`, [id])).rows[0]
    return row ? map(row) : undefined
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
    return map(row)
  }
  async emit(event: ToolEvent) {
    const call = await this.get(event.toolCallId)
    if (!call) throw new Error("ai.tool_call_not_found")
    await this.repository.appendEvent(call.runId, event.type, { toolCallId: event.toolCallId, ...event.data })
    const input = await this.pool.query<{ turn_id: string }>(`select turn_id from ai.runs where id=$1`, [call.runId])
    const turnId = input.rows[0]?.turn_id
    if (turnId) await this.repository.appendItem({
      runId: call.runId, turnId, type: event.type === "tool_call.succeeded" || event.type === "tool_call.failed" ? "tool_result" : "tool_call",
      status: event.type === "tool_call.failed" ? "failed" : "completed",
      content: { toolCallId: call.id, operationId: call.operationId, status: call.status, arguments: call.arguments, result: call.result, errorCode: call.errorCode },
    })
  }
  async countForRun(runId: string) {
    return Number((await this.pool.query<{ count: string }>(`select count(*) from ai.tool_calls where run_id=$1`, [runId])).rows[0]?.count ?? 0)
  }
}

type DbToolCall = {
  id: string; run_id: string; operation_id: string; status: ToolCallStatus; arguments: Record<string, unknown>;
  arguments_hash: string; attempt: number; row_version: number; approval_expires_at: Date | null;
  mfa_purpose: string | null; result: unknown; error_code: string | null
}
function map(row: DbToolCall): ToolCallRecord {
  return {
    id: row.id, runId: row.run_id, operationId: row.operation_id, status: row.status,
    arguments: row.arguments, argumentsHash: row.arguments_hash, attempt: row.attempt, rowVersion: row.row_version,
    ...(row.approval_expires_at ? { approvalExpiresAt: row.approval_expires_at.getTime() } : {}),
    ...(row.mfa_purpose ? { mfaPurpose: row.mfa_purpose } : {}),
    ...(row.result !== null ? { result: row.result } : {}),
    ...(row.error_code ? { errorCode: row.error_code } : {}),
  }
}
