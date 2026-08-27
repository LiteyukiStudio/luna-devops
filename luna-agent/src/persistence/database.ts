import { drizzle, type NodePgDatabase } from "drizzle-orm/node-postgres"
import { Pool, type PoolClient } from "pg"
import * as schema from "./schema/index.js"
import { agentMetrics, observeDatabasePool } from "../telemetry.js"

export type AgentDatabaseOptions = {
  maxConnections?: number
  connectionTimeoutMs?: number
  statementTimeoutMs?: number
}

class InstrumentedPool extends Pool {
  override connect(): Promise<PoolClient>
  override connect(callback: (error: Error | undefined, client: PoolClient | undefined, done: (release?: unknown) => void) => void): void
  override connect(callback?: (error: Error | undefined, client: PoolClient | undefined, done: (release?: unknown) => void) => void): Promise<PoolClient> | void {
    const startedAt = performance.now()
    if (callback) {
      super.connect((error, client, done) => {
        agentMetrics.databasePoolAcquireDuration.record((performance.now() - startedAt) / 1000, {
          outcome: error ? "failed" : "succeeded",
        })
        callback(error ?? undefined, client, done)
      })
      return
    }
    return super.connect().then((client) => {
      agentMetrics.databasePoolAcquireDuration.record((performance.now() - startedAt) / 1000, { outcome: "succeeded" })
      return client
    }, (error: unknown) => {
      agentMetrics.databasePoolAcquireDuration.record((performance.now() - startedAt) / 1000, { outcome: "failed" })
      throw error
    })
  }
}

/**
 * 数据库连接层：只负责连接池、Drizzle 实例与连接生命周期。
 * 数据库迁移继续由平台 golang-migrate 管理，本模块不执行任何 DDL。
 */
export class AgentDatabase {
  readonly pool: Pool
  readonly db: NodePgDatabase<typeof schema> & { $client: Pool }

  constructor(connectionString: string, options: AgentDatabaseOptions = {}) {
    this.pool = new InstrumentedPool({
      connectionString,
      max: options.maxConnections ?? 10,
      connectionTimeoutMillis: options.connectionTimeoutMs ?? 5_000,
      statement_timeout: options.statementTimeoutMs ?? 15_000,
      application_name: "luna-agent",
    })
    observeDatabasePool(this.pool)
    // 不启用 Drizzle 查询日志，避免 SQL 参数进入日志
    this.db = drizzle({ client: this.pool, schema })
  }

  async health(): Promise<boolean> {
    try {
      await this.pool.query("select 1")
      return true
    }
    catch {
      return false
    }
  }

  async readiness(): Promise<{ database: boolean, schema: boolean }> {
    try {
      const result = await this.pool.query<{ schema_ready: boolean }>(`
		select count(*) = 8 as schema_ready
        from information_schema.columns
        where table_schema = 'ai'
		  and (table_name, column_name) in (
			('tool_calls', 'input_mode'),
			('tool_calls', 'arguments_ciphertext'),
			('tool_calls', 'approval_decision'),
			('runs', 'actor_session_id'),
			('runs', 'execution_snapshot_ciphertext'),
			('model_credit_holds', 'max_risk_credits'),
			('model_usages', 'prompt_tokens'),
			('conversations', 'context_used_tokens')
          )
      `)
      return { database: true, schema: result.rows[0]?.schema_ready === true }
    }
    catch {
      return { database: false, schema: false }
    }
  }

  async assertReady(): Promise<void> {
    let result
    try {
      result = await this.pool.query<{ schema_ready: boolean }>(`
        select count(*) = 8 as schema_ready
        from information_schema.columns
        where table_schema = 'ai'
          and (table_name, column_name) in (
            ('tool_calls', 'input_mode'),
            ('tool_calls', 'arguments_ciphertext'),
            ('tool_calls', 'approval_decision'),
            ('runs', 'actor_session_id'),
            ('runs', 'execution_snapshot_ciphertext'),
            ('model_credit_holds', 'max_risk_credits'),
            ('model_usages', 'prompt_tokens'),
            ('conversations', 'context_used_tokens')
          )
      `)
    }
    catch (cause) {
      throw new Error("dependency.postgres.unavailable", { cause })
    }
    if (result.rows[0]?.schema_ready !== true)
      throw new Error("database.schema.invalid")
  }

  async close(): Promise<void> {
    await this.pool.end()
  }
}

export type AgentDb = AgentDatabase["db"]

/** 事务回调中的 tx 类型 */
export type AgentTx = Parameters<Parameters<AgentDb["transaction"]>[0]>[0]
