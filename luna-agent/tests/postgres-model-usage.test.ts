import { readdir, readFile } from "node:fs/promises"
import { createServer } from "node:http"
import { resolve } from "node:path"
import { afterAll, beforeAll, beforeEach, describe, expect, it } from "vitest"
import { Pool } from "pg"
import type { AIModelSnapshot } from "../src/domain.js"
import { PostgresRepository } from "../src/persistence/postgres.js"
import type { ModelCallOperation } from "../src/persistence/repository.js"
import { BudgetedModelProvider } from "../src/provider/budgeted.js"
import { OpenAIChatCompletionsProvider } from "../src/provider/openai-chat-completions.js"
import { loadTelemetryConfig } from "../src/config.js"
import { initializeTelemetry, internalSpanOptions, shutdownTelemetry, withSpan } from "../src/telemetry.js"

/** 真 PostgreSQL 测试仅操作独立的可销毁数据库。 */
const adminDatabaseUrl = process.env.AGENT_TEST_DATABASE_URL
const suite = adminDatabaseUrl ? describe : describe.skip

suite("Postgres authoritative model usage", () => {
  const databaseName = `luna_agent_usage_test_${process.pid}_${Date.now()}`
  const ownerUserId = `usr_usage_${Date.now()}`
  let adminPool: Pool
  let repository: PostgresRepository
  let providerBaseUrl = ""
  const providerServer = createServer((request, response) => {
    request.resume()
    request.once("end", () => {
      response.writeHead(200, { "content-type": "application/json", "x-request-id": "req_pg_chain" })
      response.end(JSON.stringify({
        id: "chatcmpl_pg_chain", model: "model-pg-chain",
        choices: [{ message: { content: "done" }, finish_reason: "stop" }],
        usage: { prompt_tokens: 11, completion_tokens: 4, total_tokens: 15 },
      }))
    })
  })

  beforeAll(async () => {
    if (process.env.OTEL_SMOKE === "true") initializeTelemetry(loadTelemetryConfig())
    await new Promise<void>((resolve, reject) => {
      providerServer.once("error", reject)
      providerServer.listen(0, "127.0.0.1", resolve)
    })
    const providerAddress = providerServer.address()
    if (!providerAddress || typeof providerAddress === "string") throw new Error("test_provider_address_invalid")
    providerBaseUrl = `http://127.0.0.1:${providerAddress.port}/v1`
    if (!/^luna_agent_usage_test_[0-9_]+$/.test(databaseName)) throw new Error("unsafe test database name")
    adminPool = new Pool({ connectionString: adminDatabaseUrl })
    await adminPool.query(`CREATE DATABASE "${databaseName}"`)
    const isolatedUrl = new URL(adminDatabaseUrl!)
    isolatedUrl.pathname = `/${databaseName}`
    isolatedUrl.searchParams.delete("search_path")
    const migrationPool = new Pool({ connectionString: isolatedUrl.toString() })
    try {
      const files = (await readdir(resolve(process.cwd(), "../migrations"))).filter(name => /^\d+_.+\.up\.sql$/.test(name)).sort()
      for (const file of files) await migrationPool.query(await readFile(resolve(process.cwd(), "../migrations", file), "utf8"))
      const schema = await migrationPool.query(`
        SELECT to_regclass('ai.model_credit_holds')::text AS hold,
               to_regclass('ai.model_usages')::text AS usage,
               to_regclass('ai.model_budget_reservations')::text AS legacy
      `)
      expect(schema.rows[0]).toEqual({ hold: "ai.model_credit_holds", usage: "ai.model_usages", legacy: null })
      await migrationPool.query("INSERT INTO users(id, email, name) VALUES ($1, $2, 'Usage Test')", [ownerUserId, `${ownerUserId}@example.test`])
      await migrationPool.query("INSERT INTO user_wallets(id, user_id, balance_credits) VALUES ($1, $2, 0)", [`wlt_${ownerUserId}`, ownerUserId])
    }
    finally { await migrationPool.end() }
    repository = new PostgresRepository(isolatedUrl.toString())
  }, 120_000)

  beforeEach(async () => {
    await repository.pool.query(`
      TRUNCATE ai.model_usages, ai.model_credit_holds, ai.tool_calls, ai.run_events,
               ai.items, ai.idempotency_keys, ai.conversation_summaries, ai.runs, ai.turns, ai.conversations CASCADE
    `)
    await repository.pool.query("UPDATE user_wallets SET balance_credits = 0 WHERE user_id = $1", [ownerUserId])
  })

  afterAll(async () => {
    await new Promise<void>((resolve, reject) => providerServer.close(error => error ? reject(error) : resolve()))
    if (repository) await repository.close()
    if (adminPool) {
      if (!/^luna_agent_usage_test_[0-9_]+$/.test(databaseName)) throw new Error("unsafe test database cleanup")
      await adminPool.query(`DROP DATABASE IF EXISTS "${databaseName}"`)
      await adminPool.end()
    }
    if (process.env.OTEL_SMOKE === "true") await shutdownTelemetry()
  })

  it("serializes concurrent holds so one wallet balance cannot be spent twice", async () => {
    const model = paidModel()
    const runId = await createRun(repository, ownerUserId, "race", model)
    const risk = model.maxContextTokens + model.maxOutputTokens
    await repository.pool.query("UPDATE user_wallets SET balance_credits = $1 WHERE user_id = $2", [risk, ownerUserId])

    const results = await Promise.allSettled([
      hold(repository, ownerUserId, runId, "race-a", "assistant"),
      hold(repository, ownerUserId, runId, "race-b", "assistant"),
    ])

    expect(results.filter(result => result.status === "fulfilled")).toHaveLength(1)
    expect(String((results.find(result => result.status === "rejected") as PromiseRejectedResult).reason)).toContain("ai.wallet_balance_insufficient")
  })

  it("does not create authoritative usage when Provider usage is unavailable", async () => {
    const runId = await createRun(repository, ownerUserId, "missing", freeModel())
    await hold(repository, ownerUserId, runId, "missing", "assistant")
    await repository.markModelUsageUnavailable("aihold_missing", "missing_usage", { providerRequestId: "req_missing", failureStage: "response_body" })

    const rows = await repository.pool.query<{ count: number }>("SELECT count(*)::int AS count FROM ai.model_usages")
    const holdRow = await repository.pool.query<{ state: string, reconciliation_reason: string }>("SELECT state, reconciliation_reason FROM ai.model_credit_holds WHERE id = 'aihold_missing'")
    expect(rows.rows[0]?.count).toBe(0)
    expect(holdRow.rows[0]).toEqual({ state: "reconciliation_required", reconciliation_reason: "missing_usage" })
  })

  it("never copies an expired hold into a fake usage row", async () => {
    const runId = await createRun(repository, ownerUserId, "expired", freeModel())
    await hold(repository, ownerUserId, runId, "expired", "assistant")
    await repository.pool.query("UPDATE ai.model_credit_holds SET expires_at = now() - interval '1 second' WHERE id = 'aihold_expired'")
    await hold(repository, ownerUserId, runId, "next", "assistant")

    const expired = await repository.pool.query<{ state: string, reconciliation_reason: string }>("SELECT state, reconciliation_reason FROM ai.model_credit_holds WHERE id = 'aihold_expired'")
    const usages = await repository.pool.query<{ count: number }>("SELECT count(*)::int AS count FROM ai.model_usages")
    expect(expired.rows[0]).toEqual({ state: "reconciliation_required", reconciliation_reason: "hold_expired" })
    expect(usages.rows[0]?.count).toBe(0)
  })

  it("stores official usage above the hold assumption and requires reconciliation", async () => {
    const runId = await createRun(repository, ownerUserId, "deficit", paidModel())
    await repository.pool.query("UPDATE user_wallets SET balance_credits = 100000 WHERE user_id = $1", [ownerUserId])
    await hold(repository, ownerUserId, runId, "deficit", "assistant")

    await expect(repository.recordReportedModelUsage("aihold_deficit", {
      inputTokens: 6_500, outputTokens: 1_000, totalTokens: 7_500,
      cacheReadInputTokens: 1_000, cacheWriteInputTokens: 500, reasoningOutputTokens: 250,
    }, { callType: "stream", providerRequestId: "req_deficit", responseId: "chatcmpl_deficit" }))
      .resolves.toEqual({ reconciliationRequired: true })

    const usage = await repository.pool.query<{
      prompt_tokens: string
      completion_tokens: string
      cached_prompt_tokens: string
      cache_write_prompt_tokens: string
      reasoning_completion_tokens: string
      settlement_status: string
    }>("SELECT prompt_tokens, completion_tokens, cached_prompt_tokens, cache_write_prompt_tokens, reasoning_completion_tokens, settlement_status FROM ai.model_usages WHERE credit_hold_id = 'aihold_deficit'")
    const held = await repository.pool.query<{ state: string }>("SELECT state FROM ai.model_credit_holds WHERE id = 'aihold_deficit'")
    expect(usage.rows[0]).toEqual({
      prompt_tokens: "6500",
      completion_tokens: "1000",
      cached_prompt_tokens: "1000",
      cache_write_prompt_tokens: "500",
      reasoning_completion_tokens: "250",
      settlement_status: "reconciliation_required",
    })
    expect(held.rows[0]?.state).toBe("hold_deficit")
  })

  it("persists assistant, summary, title and retries as independent attempts", async () => {
    const runId = await createRun(repository, ownerUserId, "attempts", freeModel())
    for (const operation of ["assistant", "summary", "title"] as ModelCallOperation[]) {
      const first = await hold(repository, ownerUserId, runId, `${operation}-1`, operation)
      const second = await hold(repository, ownerUserId, runId, `${operation}-2`, operation)
      expect([first.attempt, second.attempt]).toEqual([1, 2])
    }
    const rows = await repository.pool.query("SELECT operation, attempt FROM ai.model_credit_holds ORDER BY operation, attempt")
    expect(rows.rows).toHaveLength(6)
  })

  it("keeps Agent request, Provider attempt and usage persistence in one trace", async () => {
    const model = freeModel()
    const runId = await createRun(repository, ownerUserId, "trace-chain", model)
    const provider = new BudgetedModelProvider(new OpenAIChatCompletionsProvider({
      baseUrl: providerBaseUrl, apiKey: "test-key", channelAffinityEnabled: true, promptCacheKeyEnabled: false, model: "model-pg-chain", timeoutMs: 5_000,
    }), repository)

    const result = await withSpan("agent.request", internalSpanOptions(), () => provider.complete({
      messages: [{ role: "user", content: "trace chain" }], maxOutputTokens: 32,
      budget: { runId, ownerUserId, operation: "assistant" },
    }))

    expect(result.usage).toEqual({ status: "reported", value: { inputTokens: 11, outputTokens: 4, totalTokens: 15 } })
    const usage = await repository.pool.query("SELECT prompt_tokens, completion_tokens, total_tokens FROM ai.model_usages WHERE run_id = $1", [runId])
    expect(usage.rows).toEqual([{ prompt_tokens: "11", completion_tokens: "4", total_tokens: "15" }])
    const run = await repository.getRun(ownerUserId, runId)
    const timeline = await repository.getTimeline(ownerUserId, run!.conversationId)
    expect(timeline?.contextUsage).toMatchObject({
      status: "reported", runId, modelId: model.id, usedTokens: 15,
      maxContextTokensSnapshot: model.maxContextTokens,
    })
    expect(Number.isFinite(Date.parse(timeline!.contextUsage!.recordedAt))).toBe(true)
  })
})

function freeModel(): AIModelSnapshot {
  return {
    id: "aimod_free", name: "free", maxContextTokens: 4_096, maxOutputTokens: 512,
    inputCreditsPerMillion: "0", outputCreditsPerMillion: "0", cachedInputCreditsPerMillion: "0",
  }
}

function paidModel(): AIModelSnapshot {
  return { ...freeModel(), id: "aimod_paid", name: "paid", inputCreditsPerMillion: "1000000", outputCreditsPerMillion: "1000000" }
}

async function createRun(repository: PostgresRepository, ownerUserId: string, suffix: string, model: AIModelSnapshot) {
  const conversation = await repository.createConversation(ownerUserId, suffix)
  const created = await repository.createTurn(ownerUserId, {
    conversationId: conversation.id, input: suffix, pageContext: {}, idempotencyKey: `usage-${suffix}`,
    actorSessionId: `session-${suffix}`, modelId: model.id, modelSnapshot: model,
  })
  return created.run.id
}

function hold(repository: PostgresRepository, ownerUserId: string, runId: string, suffix: string, operation: ModelCallOperation) {
  return repository.createModelCreditHold({
    id: `aihold_${suffix}`, runId, ownerUserId, operation, requestedOutputTokens: 512, leaseSeconds: 60,
  })
}
