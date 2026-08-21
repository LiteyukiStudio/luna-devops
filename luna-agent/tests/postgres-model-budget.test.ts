import { readdir, readFile } from "node:fs/promises"
import { resolve } from "node:path"
import { afterAll, beforeAll, beforeEach, describe, expect, it } from "vitest"
import { Pool } from "pg"
import type { AIModelSnapshot } from "../src/domain.js"
import { PostgresRepository } from "../src/persistence/postgres.js"
import type { ModelBudgetOperation } from "../src/persistence/repository.js"

/**
 * 真 PostgreSQL 预算并发测试。测试会从 AGENT_TEST_DATABASE_URL 连接管理库，
 * 创建 luna_agent_budget_test_* 独立数据库、应用全部 migration，并在结束时
 * 关闭全部连接后删除该数据库；没有显式环境变量时不会接触任何数据库。
 */
const adminDatabaseUrl = process.env.AGENT_TEST_DATABASE_URL
const suite = adminDatabaseUrl ? describe : describe.skip

suite("Postgres model budget reservations", () => {
  const databaseName = `luna_agent_budget_test_${process.pid}_${Date.now()}`
  const ownerUserId = `usr_budget_${Date.now()}`
  let adminPool: Pool
  let repository: PostgresRepository

  beforeAll(async () => {
    if (!/^luna_agent_budget_test_[0-9_]+$/.test(databaseName)) throw new Error("unsafe test database name")
    adminPool = new Pool({ connectionString: adminDatabaseUrl })
    await adminPool.query(`CREATE DATABASE "${databaseName}"`)
    const isolatedUrl = new URL(adminDatabaseUrl!)
    isolatedUrl.pathname = `/${databaseName}`
    isolatedUrl.searchParams.delete("search_path")
    const migrationPool = new Pool({ connectionString: isolatedUrl.toString() })
    try {
      const migrationDirectory = resolve(process.cwd(), "../migrations")
      const files = (await readdir(migrationDirectory)).filter(name => /^\d+_.+\.up\.sql$/.test(name)).sort()
      for (const file of files) await migrationPool.query(await readFile(resolve(migrationDirectory, file), "utf8"))
      const budgetUp = await readFile(resolve(migrationDirectory, "000077_ai_model_run_budgets.up.sql"), "utf8")
      const budgetDown = await readFile(resolve(migrationDirectory, "000077_ai_model_run_budgets.down.sql"), "utf8")
      await migrationPool.query(budgetDown)
      const rolledBack = await migrationPool.query<{ reservation: string | null, model_cap: boolean }>(`
        SELECT to_regclass('ai.model_budget_reservations')::text AS reservation,
               EXISTS (
                 SELECT 1 FROM information_schema.columns
                 WHERE table_schema = 'public' AND table_name = 'ai_models' AND column_name = 'max_context_tokens'
               ) AS model_cap
      `)
      expect(rolledBack.rows[0]).toEqual({ reservation: null, model_cap: false })
      await migrationPool.query(budgetUp)
      await migrationPool.query(
        "INSERT INTO users(id, email, name) VALUES ($1, $2, 'Budget Test')",
        [ownerUserId, `${ownerUserId}@example.test`],
      )
      await migrationPool.query(
        "INSERT INTO user_wallets(id, user_id, balance_credits) VALUES ($1, $2, 0)",
        [`wlt_${ownerUserId}`, ownerUserId],
      )
    }
    finally {
      await migrationPool.end()
    }
    repository = new PostgresRepository(isolatedUrl.toString())
  }, 120_000)

  beforeEach(async () => {
    await repository.pool.query(`
      TRUNCATE ai.model_budget_reservations, ai.ui_actions, ai.tool_calls, ai.run_events,
               ai.items, ai.idempotency_keys, ai.conversation_summaries, ai.runs,
               ai.turns, ai.conversations CASCADE
    `)
    await repository.pool.query("UPDATE user_wallets SET balance_credits = 0 WHERE user_id = $1", [ownerUserId])
  })

  afterAll(async () => {
    if (repository) await repository.close()
    if (adminPool) {
      if (!/^luna_agent_budget_test_[0-9_]+$/.test(databaseName)) throw new Error("unsafe test database cleanup")
      // A normal DROP is deliberate: if any repository client or in-flight
      // reservation still owns a connection, cleanup must fail instead of
      // force-terminating it and producing a false-green unhandled 57P01.
      await adminPool.query(`DROP DATABASE IF EXISTS "${databaseName}"`)
      await adminPool.end()
    }
  })

  it("allows zero-priced calls with an empty wallet and honors the exact paid boundary", async () => {
    const freeRun = await createBudgetRun(repository, ownerUserId, "free", zeroPriceModel(), 100, "100")
    await expect(reserve(repository, ownerUserId, freeRun, "free", 5, 5)).resolves.toMatchObject({ maxOutputTokens: 5 })

    const paid = { ...zeroPriceModel(), id: "aimod_paid", name: "paid", inputCreditsPerMillion: "1000000", outputCreditsPerMillion: "1000000" }
    const paidRun = await createBudgetRun(repository, ownerUserId, "paid", paid, 100, "10")
    await expect(reserve(repository, ownerUserId, paidRun, "paid-empty", 5, 5)).rejects.toThrow("ai.wallet_balance_insufficient")
    await repository.pool.query("UPDATE user_wallets SET balance_credits = 10 WHERE user_id = $1", [ownerUserId])
    await expect(reserve(repository, ownerUserId, paidRun, "paid-exact", 5, 5)).resolves.toMatchObject({ maxOutputTokens: 5 })
  })

  it("serializes concurrent AI reservations so they cannot spend the same wallet balance", async () => {
    const paid = { ...zeroPriceModel(), id: "aimod_race", name: "race", inputCreditsPerMillion: "1000000", outputCreditsPerMillion: "1000000" }
    const runId = await createBudgetRun(repository, ownerUserId, "race", paid, 100, "100")
    await repository.pool.query("UPDATE user_wallets SET balance_credits = 10 WHERE user_id = $1", [ownerUserId])

    const results = await Promise.allSettled([
      reserve(repository, ownerUserId, runId, "race-a", 5, 5),
      reserve(repository, ownerUserId, runId, "race-b", 5, 5),
    ])

    expect(results.filter(result => result.status === "fulfilled")).toHaveLength(1)
    const rejected = results.find(result => result.status === "rejected") as PromiseRejectedResult
    expect(String(rejected.reason)).toContain("ai.wallet_balance_insufficient")
  })

  it("recovers expired reservations conservatively and settled usage remains in the Run budget", async () => {
    const runId = await createBudgetRun(repository, ownerUserId, "stale", zeroPriceModel(), 20, "100")
    await reserve(repository, ownerUserId, runId, "stale-old", 5, 5)
    await repository.pool.query(
      "UPDATE ai.model_budget_reservations SET expires_at = now() - interval '1 second' WHERE id = $1",
      ["aibgt_stale-old"],
    )

    const next = await reserve(repository, ownerUserId, runId, "stale-next", 5, 10)
    expect(next.maxOutputTokens).toBe(5)
    const recovered = await repository.pool.query(
      "SELECT state, confirmed_tokens, input_tokens, output_tokens FROM ai.model_budget_reservations WHERE id = $1",
      ["aibgt_stale-old"],
    )
    expect(recovered.rows[0]).toMatchObject({ state: "confirmed", confirmed_tokens: "10", input_tokens: "5", output_tokens: "5" })

    await repository.releaseModelBudget("aibgt_stale-next")
    await repository.pool.query("UPDATE ai.model_budget_reservations SET state = 'settled' WHERE id = $1", ["aibgt_stale-old"])
    await repository.pool.query("UPDATE ai.runs SET total_token_budget = 10 WHERE id = $1", [runId])
    await expect(reserve(repository, ownerUserId, runId, "after-settlement", 1, 1)).rejects.toThrow("ai.run_token_budget_exhausted")
  })

  it("does not deadlock expired recovery against the wallet-first settlement lock path", async () => {
    const runId = await createBudgetRun(repository, ownerUserId, "lock-order", zeroPriceModel(), 100, "100")
    await reserve(repository, ownerUserId, runId, "expired-race", 5, 5)
    await repository.pool.query(
      "UPDATE ai.model_budget_reservations SET expires_at = now() - interval '1 second' WHERE id = $1",
      ["aibgt_expired-race"],
    )

    const settlement = await repository.pool.connect()
    try {
      await settlement.query("BEGIN")
      await settlement.query("SELECT id FROM user_wallets WHERE user_id = $1 FOR UPDATE", [ownerUserId])
      const recovery = reserve(repository, ownerUserId, runId, "after-lock-race", 5, 5)
      // New wallet-first recovery blocks before touching the reservation. The old
      // reservation-first order would hold this row while waiting for our wallet.
      await new Promise(resolve => setTimeout(resolve, 50))
      await settlement.query("SET LOCAL lock_timeout = '250ms'")
      await settlement.query("SELECT id FROM ai.model_budget_reservations WHERE id = $1 FOR UPDATE", ["aibgt_expired-race"])
      await settlement.query(`
        UPDATE ai.model_budget_reservations
        SET state = 'settled', confirmed_tokens = reserved_tokens,
            confirmed_credits = reserved_credits, input_tokens = reserved_input_tokens,
            output_tokens = reserved_output_tokens, cached_input_tokens = 0,
            cached_output_tokens = 0, updated_at = now()
        WHERE id = $1
      `, ["aibgt_expired-race"])
      await settlement.query("COMMIT")
      await expect(Promise.race([
        recovery,
        new Promise((_, reject) => setTimeout(() => reject(new Error("budget lock timeout")), 2_000)),
      ])).resolves.toMatchObject({ maxOutputTokens: 5 })
    }
    finally {
      await settlement.query("ROLLBACK").catch(() => undefined)
      settlement.release()
    }
  })

  it("persists every model operation in the same authoritative reservation source", async () => {
    const runId = await createBudgetRun(repository, ownerUserId, "operations", zeroPriceModel(), 1000, "100")
    const operations: ModelBudgetOperation[] = ["assistant", "summary", "title"]
    for (const operation of operations) {
      await repository.reserveModelBudget({
        id: `aibgt_${operation}`, runId, ownerUserId, operation,
        estimatedInputTokens: 5, requestedOutputTokens: 5, leaseSeconds: 60,
      })
      await repository.confirmModelBudget(`aibgt_${operation}`, { inputTokens: 4, outputTokens: 3, cachedInputTokens: 2, cachedOutputTokens: 1, reported: true })
    }
    const rows = await repository.pool.query<{ operation: string }>("SELECT operation FROM ai.model_budget_reservations ORDER BY operation")
    expect(rows.rows.map(row => row.operation)).toEqual([...operations].sort())
  })

  it("rejects reported usage above the hold and can conservatively confirm the reservation", async () => {
    const runId = await createBudgetRun(repository, ownerUserId, "usage-over-hold", zeroPriceModel(), 100, "100")
    await reserve(repository, ownerUserId, runId, "usage-over-hold", 5, 5)
    await expect(repository.confirmModelBudget("aibgt_usage-over-hold", {
      inputTokens: 6, outputTokens: 5, reported: true,
    })).rejects.toThrow("ai.provider_usage_invalid")
    const beforeFallback = await repository.pool.query<{ state: string }>(
      "SELECT state FROM ai.model_budget_reservations WHERE id = $1", ["aibgt_usage-over-hold"],
    )
    expect(beforeFallback.rows[0]?.state).toBe("reserved")
    await repository.confirmModelBudget("aibgt_usage-over-hold")
    const fallback = await repository.pool.query<{ state: string, confirmed_tokens: string }>(
      "SELECT state, confirmed_tokens FROM ai.model_budget_reservations WHERE id = $1", ["aibgt_usage-over-hold"],
    )
    expect(fallback.rows[0]).toEqual({ state: "confirmed", confirmed_tokens: "10" })
  })
})

function zeroPriceModel(): AIModelSnapshot {
  return {
    id: "aimod_free", name: "free", maxContextTokens: 4096, maxOutputTokens: 512,
    inputCreditsPerMillion: "0", outputCreditsPerMillion: "0",
    cachedInputCreditsPerMillion: "0", cachedOutputCreditsPerMillion: "0",
  }
}

async function createBudgetRun(repository: PostgresRepository, ownerUserId: string, suffix: string, model: AIModelSnapshot, totalTokens: number, totalCredits: string) {
  const conversation = await repository.createConversation(ownerUserId, suffix)
  const created = await repository.createTurn(ownerUserId, {
    conversationId: conversation.id,
    input: suffix,
    pageContext: {},
    idempotencyKey: `budget-${suffix}`,
    actorSessionId: `session-${suffix}`,
    modelId: model.id,
    modelSnapshot: model,
    runBudgetSnapshot: { totalTokens, totalCredits },
  })
  return created.run.id
}

function reserve(repository: PostgresRepository, ownerUserId: string, runId: string, suffix: string, inputTokens: number, outputTokens: number) {
  return repository.reserveModelBudget({
    id: `aibgt_${suffix}`, runId, ownerUserId,
    operation: "assistant", estimatedInputTokens: inputTokens, requestedOutputTokens: outputTokens, leaseSeconds: 60,
  })
}
