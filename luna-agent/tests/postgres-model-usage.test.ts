import { createServer, type Server } from "node:http"
import type { AddressInfo } from "node:net"
import { readdir, readFile } from "node:fs/promises"
import { resolve } from "node:path"
import { Pool } from "pg"
import { afterAll, beforeAll, beforeEach, describe, expect, it } from "vitest"
import type { AIModelSnapshot } from "../src/domain.js"
import { loadConfig } from "../src/config.js"
import { PostgresRepository } from "../src/persistence/postgres.js"
import type { ModelCallOperation } from "../src/persistence/repository.js"
import { OpenAIChatCompletionsProvider } from "../src/provider/openai-chat-completions.js"
import { UsageRecordingModelProvider } from "../src/provider/usage-recorder.js"
import { configureAgentTelemetry, internalSpanOptions, shutdownTelemetry, withSpan } from "../src/telemetry.js"

const databaseUrl = process.env.AGENT_TEST_DATABASE_URL
const suite = databaseUrl ? describe : describe.skip

suite("PostgreSQL AI model usage ledger and positive-balance admission", () => {
  let adminPool: Pool
  let databaseName: string
  let repository: PostgresRepository
  let providerServer: Server
  let providerBaseUrl: string
  const ownerUserId = "usr_agent_usage_test"

  beforeAll(async () => {
    if (process.env.OTEL_SMOKE === "true") {
      configureAgentTelemetry(loadConfig({
        NODE_ENV: "test",
        OTEL_SERVICE_NAME: "luna-agent-usage-smoke",
      }))
    }
    providerServer = createServer((_request, response) => {
      response.writeHead(200, { "content-type": "application/json", "x-request-id": "req_pg_chain" })
      response.end(JSON.stringify({
        id: "chatcmpl_pg_chain", model: "model-pg-chain",
        choices: [{ message: { content: "ok" }, finish_reason: "stop" }],
        usage: { prompt_tokens: 11, completion_tokens: 4, total_tokens: 15 },
      }))
    })
    await new Promise<void>(resolveListen => providerServer.listen(0, "127.0.0.1", () => resolveListen()))
    providerBaseUrl = `http://127.0.0.1:${(providerServer.address() as AddressInfo).port}/v1`

    adminPool = new Pool({ connectionString: databaseUrl })
    databaseName = `luna_agent_usage_test_${Date.now()}_${process.pid}`
    await adminPool.query(`CREATE DATABASE "${databaseName}"`)
    const isolatedUrl = new URL(databaseUrl!)
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
      expect(schema.rows[0]).toEqual({ hold: null, usage: "ai.model_usages", legacy: null })
      await migrationPool.query("INSERT INTO users(id, email, name) VALUES ($1, $2, 'Usage Test')", [ownerUserId, `${ownerUserId}@example.test`])
      await migrationPool.query("INSERT INTO user_wallets(id, user_id, balance_credits) VALUES ($1, $2, 10)", [`wlt_${ownerUserId}`, ownerUserId])
    }
    finally { await migrationPool.end() }
    repository = new PostgresRepository(isolatedUrl.toString())
  }, 120_000)

  beforeEach(async () => {
    await repository.pool.query(`
      TRUNCATE ai.model_usages, ai.tool_calls, ai.run_events,
               ai.items, ai.idempotency_keys, ai.conversation_summaries, ai.runs, ai.turns, ai.conversations CASCADE
    `)
    await repository.pool.query("UPDATE user_wallets SET balance_credits = 10 WHERE user_id = $1", [ownerUserId])
  })

  afterAll(async () => {
    await new Promise<void>((resolveClose, reject) => providerServer.close(error => error ? reject(error) : resolveClose()))
    if (repository) await repository.close()
    if (adminPool) {
      if (!/^luna_agent_usage_test_[0-9_]+$/.test(databaseName)) throw new Error("unsafe test database cleanup")
      await adminPool.query(`DROP DATABASE IF EXISTS "${databaseName}"`)
      await adminPool.end()
    }
    if (process.env.OTEL_SMOKE === "true") await shutdownTelemetry()
  })

  it("allows small-balance users on 1M-context / 64K-output models while blocking zero or negative available balance", async () => {
    // Small balance ($0.50) would have failed full-window pre-reservation, but passes lock-free positive-balance gate.
    await repository.pool.query("UPDATE user_wallets SET balance_credits = 0.5 WHERE user_id = $1", [ownerUserId])
    const largeWindowModel: AIModelSnapshot = {
      id: "aimod_1m",
      name: "1m-model",
      maxContextTokens: 1_000_000,
      maxOutputTokens: 65_536,
      inputCreditsPerMillion: "5000",
      outputCreditsPerMillion: "20000",
      cachedInputCreditsPerMillion: "1000",
    }
    const runId = await createRun(repository, ownerUserId, "large-window", largeWindowModel)
    await repository.recordReportedModelUsage({
      runId,
      ownerUserId,
      operation: "assistant",
      usage: { inputTokens: 100_000, outputTokens: 20_000, totalTokens: 120_000 },
      callType: "stream",
    })
    // Pending AI usage (0.5 + 0.4 = 0.9) now exceeds 0.50 wallet balance => available <= 0 => next Turn is rejected!
    await expect(createRun(repository, ownerUserId, "overdraft-next-turn", largeWindowModel))
      .rejects.toThrow("ai.wallet_balance_insufficient")
  })

  it("persists assistant, summary, title and retries as independent attempts with normalized gateway tokens", async () => {
    const model = paidModel()
    const runId = await createRun(repository, ownerUserId, "attempts", model)
    for (const operation of ["assistant", "summary", "title"] as ModelCallOperation[]) {
      const first = await repository.recordReportedModelUsage({
        runId,
        ownerUserId,
        operation,
        usage: {
          inputTokens: 300,
          outputTokens: 50,
          totalTokens: 420,
          cacheReadInputTokens: 200,
          cacheWriteInputTokens: 20,
          reasoningOutputTokens: 70,
        },
        callType: operation === "assistant" ? "stream" : "complete",
      })
      const second = await repository.recordReportedModelUsage({
        runId,
        ownerUserId,
        operation,
        usage: { inputTokens: 100, outputTokens: 10, totalTokens: 110 },
        callType: "complete",
      })
      expect([first.attempt, second.attempt]).toEqual([1, 2])
    }
    const rows = await repository.pool.query("SELECT operation, attempt FROM ai.model_usages ORDER BY operation, attempt")
    expect(rows.rows).toHaveLength(6)
  })

  it("cancels active runs and preserves immutable ai.model_usages when deleting a conversation", async () => {
    const model = paidModel()
    const conversation = await repository.createConversation(ownerUserId, "delete-keep-ledger")
    const created = await repository.createTurn(ownerUserId, {
      conversationId: conversation.id,
      input: "stream and delete",
      pageContext: {},
      idempotencyKey: "usage-delete-keep-ledger",
      actorSessionId: "session-delete",
      modelId: model.id,
      modelSnapshot: model,
    })
    await repository.recordReportedModelUsage({
      runId: created.run.id,
      ownerUserId,
      operation: "assistant",
      usage: { inputTokens: 500, outputTokens: 100, totalTokens: 600 },
      callType: "stream",
    })

    await expect(repository.deleteConversation(ownerUserId, conversation.id)).resolves.toBe(true)
    const remainingUsages = await repository.pool.query<{ count: number }>(
      "SELECT count(*)::int AS count FROM ai.model_usages WHERE run_id = $1",
      [created.run.id],
    )
    expect(remainingUsages.rows[0]?.count).toBe(1)
  })

  it("keeps Agent request, Provider attempt and usage persistence in one trace", async () => {
    const model = freeModel()
    const runId = await createRun(repository, ownerUserId, "trace-chain", model)
    const provider = new UsageRecordingModelProvider(new OpenAIChatCompletionsProvider({
      baseUrl: providerBaseUrl, apiKey: "test-key", channelAffinityEnabled: true, promptCacheKeyEnabled: false, model: "model-pg-chain", timeoutMs: 5_000,
    }), repository)

    const result = await withSpan("agent.request", internalSpanOptions(), () => provider.complete({
      messages: [{ role: "user", content: "trace chain" }], maxOutputTokens: 32,
      modelPricing: model,
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
