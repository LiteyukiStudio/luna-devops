import { drizzle, type NodePgDatabase } from "drizzle-orm/node-postgres"
import { Pool } from "pg"
import * as schema from "./schema/index.js"

/**
 * 数据库连接层：只负责连接池、Drizzle 实例与连接生命周期。
 * 数据库迁移继续由平台 golang-migrate 管理，本模块不执行任何 DDL。
 */
export class AgentDatabase {
  readonly pool: Pool
  readonly db: NodePgDatabase<typeof schema> & { $client: Pool }

  constructor(connectionString: string) {
    this.pool = new Pool({ connectionString, max: 10, application_name: "luna-agent" })
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
		select count(*) = 4 as schema_ready
        from information_schema.columns
        where table_schema = 'ai'
		  and (table_name, column_name) in (
			('tool_calls', 'input_mode'),
			('tool_calls', 'arguments_ciphertext'),
			('tool_calls', 'approval_decision'),
			('runs', 'actor_session_id')
          )
      `)
      return { database: true, schema: result.rows[0]?.schema_ready === true }
    }
    catch {
      return { database: false, schema: false }
    }
  }

  async close(): Promise<void> {
    await this.pool.end()
  }
}

export type AgentDb = AgentDatabase["db"]

/** 事务回调中的 tx 类型 */
export type AgentTx = Parameters<Parameters<AgentDb["transaction"]>[0]>[0]
