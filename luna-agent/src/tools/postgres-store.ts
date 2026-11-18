import type { Pool } from "pg"
import type { Run } from "../domain.js"
import type { PayloadCipher } from "../payload-cipher.js"
import { RunStateConflictError, type Repository } from "../persistence/repository.js"
import { activeRunningRunFence, activeRunningRunVersionFence } from "../persistence/run-fence.js"
import { redact } from "../redaction.js"
import type { ToolCallRecord, ToolCallStatus, ToolCallStore, ToolEvent } from "./orchestrator.js"

export class PostgresToolCallStore implements ToolCallStore {
  constructor(
    private readonly pool: Pool,
    private readonly repository: Repository,
    private readonly argumentsCipher: PayloadCipher,
  ) {}
  async insert(value: ToolCallRecord, expectedRunVersion?: number) {
    const effectiveVersion = expectedRunVersion ?? activeRunningRunVersionFence(value.runId)
    const params = [
      value.id,
      value.idempotencyKey,
      value.verificationForToolCallId ?? "",
      value.runId,
      value.operationId,
      value.status,
      value.inputMode ?? "model",
      JSON.stringify(redact(value.arguments)),
      this.argumentsCipher.encrypt(JSON.stringify(value.arguments), `${value.runId}:${value.id}`),
      value.argumentsHash,
      value.attempt,
      value.rowVersion,
      value.approvalDecision ?? null,
      value.result === undefined ? null : JSON.stringify(value.result),
      value.errorCode ?? null,
      value.executionCount,
    ]
    try {
      if (effectiveVersion === undefined) {
        await this.pool.query(
          `insert into ai.tool_calls(id,idempotency_key,verification_for_tool_call_id,run_id,operation_id,status,input_mode,arguments,arguments_ciphertext,arguments_hash,attempt,row_version,approval_decision,result,error_code,execution_count)
           values($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
          params,
        )
        return
      }
      const inserted = await this.pool.query<{ id: string }>(
        `with active_run as (
           select id from ai.runs
           where id = $4 and status = 'running' and row_version = $17
           for update
         )
         insert into ai.tool_calls(id,idempotency_key,verification_for_tool_call_id,run_id,operation_id,status,input_mode,arguments,arguments_ciphertext,arguments_hash,attempt,row_version,approval_decision,result,error_code,execution_count)
         select $1,$2,$3,active_run.id,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16
         from active_run
         returning id`,
        [...params, effectiveVersion],
      )
      if (inserted.rows.length === 0) {
        const current = (await this.pool.query<{ status: Run["status"], row_version: number }>(
          `select status, row_version from ai.runs where id = $1`,
          [value.runId],
        )).rows[0]
        throw new RunStateConflictError(value.runId, "running", "running", current?.status, current?.row_version)
      }
    }
    catch (error) {
      if (error instanceof RunStateConflictError) throw error
      throw toolPersistenceError(error)
    }
  }
  async get(id: string) {
    const row = (await this.pool.query<DbToolCall>(`select * from ai.tool_calls where id=$1`, [id])).rows[0]
    return row ? this.map(row) : undefined
  }
  async listByRun(runId: string) {
    try {
      const rows = (await this.pool.query<DbToolCall>(
        `select * from ai.tool_calls where run_id=$1 order by created_at, id`,
        [runId],
      )).rows
      return rows.map(row => this.map(row))
    }
    catch (error) {
      throw toolPersistenceError(error)
    }
  }
  async update(id: string, expected: ToolCallStatus, patch: Partial<ToolCallRecord>, expectedRunVersion?: number) {
    const activeFence = activeRunningRunFence()
    const effectiveVersion = expectedRunVersion ?? activeFence?.expectedRunVersion
    const params = [
      id,
      expected,
      patch.status ?? null,
      patch.rowVersion ?? null,
      patch.approvalDecision ?? null,
      patch.result === undefined ? null : JSON.stringify(patch.result),
      patch.errorCode ?? null,
      patch.executionCount ?? null,
    ]
    if (effectiveVersion === undefined) {
      const row = (await this.pool.query<DbToolCall>(
        `update ai.tool_calls set status=coalesce($3,status),row_version=coalesce($4,row_version),
         approval_decision=coalesce($5,approval_decision),
         result=coalesce($6,result),error_code=coalesce($7,error_code),execution_count=coalesce($8,execution_count),updated_at=now()
         where id=$1 and status=$2 returning *`,
        params,
      )).rows[0]
      if (!row) throw new Error("ai.tool_call_state_conflict")
      return this.map(row)
    }
    const row = (await this.pool.query<DbToolCall>(
      `with active_run as (
         select r.id from ai.runs r
         join ai.tool_calls tc on tc.run_id = r.id
         where tc.id = $1 and r.status = 'running' and r.row_version = $9
         for update of r
       )
       update ai.tool_calls set status=coalesce($3,status),row_version=coalesce($4,row_version),
       approval_decision=coalesce($5,approval_decision),
       result=coalesce($6,result),error_code=coalesce($7,error_code),execution_count=coalesce($8,execution_count),updated_at=now()
       from active_run
       where ai.tool_calls.id=$1 and ai.tool_calls.status=$2 and ai.tool_calls.run_id=active_run.id
       returning ai.tool_calls.*`,
      [...params, effectiveVersion],
    )).rows[0]
    if (!row) {
      const current = (await this.pool.query<{ run_id: string, run_status: Run["status"], row_version: number }>(
        `select tc.run_id, r.status as run_status, r.row_version
         from ai.tool_calls tc
         join ai.runs r on r.id = tc.run_id
         where tc.id = $1`,
        [id],
      )).rows[0]
      if (current && (current.run_status !== "running" || current.row_version !== effectiveVersion)) {
        throw new RunStateConflictError(current.run_id, "running", "running", current.run_status, current.row_version)
      }
      throw new Error("ai.tool_call_state_conflict")
    }
    return this.map(row)
  }
  async emit(event: ToolEvent) {
    const call = await this.get(event.toolCallId)
    if (!call) throw new Error("ai.tool_call_not_found")
    const input = await this.pool.query<{ turn_id: string }>(`select turn_id from ai.runs where id=$1`, [call.runId])
    const turnId = input.rows[0]?.turn_id
    if (!turnId) return
    const itemId = `${call.id}:item`
    const existingItem = event.type === "tool.started"
      ? undefined
      : (await this.pool.query<{ content: Record<string, unknown> }>(`select content from ai.items where id=$1`, [itemId])).rows[0]?.content
    const modelCallId = typeof event.data.modelCallId === "string"
      ? event.data.modelCallId
      : typeof existingItem?.modelCallId === "string"
        ? existingItem.modelCallId
        : undefined
    const batchId = typeof event.data.batchId === "string"
      ? event.data.batchId
      : typeof existingItem?.batchId === "string"
        ? existingItem.batchId
        : undefined
    const content = {
        toolCallId: call.id,
        ...(modelCallId ? { modelCallId } : {}),
        ...(batchId ? { batchId } : {}),
        operationId: call.operationId, status: call.status, inputMode: call.inputMode ?? "model",
        ...(call.verificationForToolCallId ? { verificationForToolCallId: call.verificationForToolCallId } : {}),
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
    const argumentsValue = JSON.parse(this.argumentsCipher.decrypt(row.arguments_ciphertext, `${row.run_id}:${row.id}`)) as Record<string, unknown>
    return {
      id: row.id, idempotencyKey: row.idempotency_key || row.id, runId: row.run_id, operationId: row.operation_id, status: row.status,
      ...(row.verification_for_tool_call_id ? { verificationForToolCallId: row.verification_for_tool_call_id } : {}),
      arguments: argumentsValue, argumentsHash: row.arguments_hash, attempt: row.attempt, rowVersion: row.row_version, executionCount: row.execution_count,
      ...(row.input_mode === "direct" || row.input_mode === "model" || row.input_mode === "verification" ? { inputMode: row.input_mode } : {}),
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
  id: string; idempotency_key: string; verification_for_tool_call_id: string; run_id: string; operation_id: string; status: ToolCallStatus; input_mode: string; arguments: Record<string, unknown>;
  arguments_ciphertext: string | null;
  arguments_hash: string; attempt: number; row_version: number; execution_count: number;
	approval_decision: "approve" | null;
	result: unknown; error_code: string | null
}

function toolPersistenceError(error: unknown): Error {
  const postgresCode = error && typeof error === "object" && "code" in error
    ? String(error.code)
    : ""
  return new Error(postgresCode === "42703" ? "ai.database_schema_mismatch" : "ai.tool_persistence_failed")
}
