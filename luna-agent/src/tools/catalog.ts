import { createHash } from "node:crypto"
import { z } from "zod"

export const toolRisk = z.enum(["read", "ui", "write", "sensitive", "destructive"])
export type ToolRisk = z.infer<typeof toolRisk>
const jsonSchema = z.object({
  type: z.literal("object"),
  properties: z.record(z.string(), z.object({
    type: z.enum(["string", "number", "integer", "boolean", "array", "object"]),
    maxLength: z.number().int().positive().optional(),
    maximum: z.number().optional(),
    enum: z.array(z.union([z.string(), z.number(), z.boolean()])).optional(),
  })).default({}),
  required: z.array(z.string()).default([]),
  additionalProperties: z.literal(false),
})

const operation = z.object({
  operationId: z.string().regex(/^[A-Za-z][A-Za-z0-9._-]{2,100}$/),
  method: z.enum(["GET", "POST", "PUT", "PATCH", "DELETE"]),
  path: z.string().startsWith("/api/v1/"),
  category: z.string().min(1),
  risk: toolRisk,
  requiredScopes: z.array(z.string()).max(20),
  approval: z.enum(["never", "always", "risk_based"]),
  stepUpPurpose: z.string().optional(),
  idempotent: z.boolean(),
  timeoutMs: z.number().int().min(100).max(120000),
  inputSchema: jsonSchema,
  maxItems: z.number().int().min(1).max(500).optional(),
  resultVerifier: z.string().optional(),
}).superRefine((value, context) => {
  if (["sensitive", "destructive"].includes(value.risk) && !value.stepUpPurpose) {
    context.addIssue({ code: "custom", message: "high-risk operation requires stepUpPurpose" })
  }
  if (["sensitive", "destructive"].includes(value.risk) && value.approval === "never") {
    context.addIssue({ code: "custom", message: "high-risk operation requires approval" })
  }
  if (value.risk !== "read" && !value.idempotent) {
    context.addIssue({ code: "custom", message: "write operation requires idempotency" })
  }
})

export type ToolOperation = z.infer<typeof operation>

const platformContextOperations = new Set(["getDashboard", "listProjects", "listAppTemplates", "createProject"])

export class ToolCatalog {
  private readonly operations: Map<string, ToolOperation>
  readonly digest: string
  private constructor(values: ToolOperation[]) {
    this.operations = new Map(values.map(value => [value.operationId, value]))
    if (this.operations.size !== values.length) throw new Error("ai.tool_catalog_duplicate_operation")
    this.digest = `sha256:${createHash("sha256").update(JSON.stringify([...values].sort((a, b) => a.operationId.localeCompare(b.operationId)))).digest("hex")}`
  }
  static load(input: unknown): ToolCatalog {
    return new ToolCatalog(z.array(operation).min(1).parse(input))
  }
  get(operationId: string): ToolOperation {
    const value = this.operations.get(operationId)
    if (!value) throw new Error("ai.tool_not_available")
    return value
  }
  all(): ToolOperation[] {
    return [...this.operations.values()]
  }
  modelTools(context: { projectId?: string } = {}) {
    void context
    return this.all()
      .map(item => ({
        operationId: item.operationId,
        description: `${item.category} 类操作，风险级别为 ${item.risk}。${platformContextOperations.has(item.operationId) ? "该操作作用于平台范围，不能传入 projectId。" : "必须使用从用户可见资源中明确选择的 projectId；页面上下文只提供指引，不代表授权。"}只有用户需要查询当前 Luna DevOps 数据或明确执行平台操作时才可使用。`,
        inputSchema: item.inputSchema,
      }))
  }
  select(category: string, limit = 15): ToolOperation[] {
    return [...this.operations.values()].filter(item => item.category === category).slice(0, Math.min(15, limit))
  }
}

export function validateArguments(schema: ToolOperation["inputSchema"], input: unknown): Record<string, unknown> {
  if (!input || typeof input !== "object" || Array.isArray(input)) throw new Error("ai.tool_arguments_invalid")
  const value = input as Record<string, unknown>
  const allowed = new Set(Object.keys(schema.properties))
  if (Object.keys(value).some(key => !allowed.has(key)) || schema.required.some(key => value[key] === undefined)) throw new Error("ai.tool_arguments_invalid")
  for (const [key, item] of Object.entries(value)) {
    const rule = schema.properties[key]
    if (!rule || !matches(rule.type, item)) throw new Error("ai.tool_arguments_invalid")
    if (typeof item === "string" && rule.maxLength && item.length > rule.maxLength) throw new Error("ai.tool_arguments_invalid")
    if (typeof item === "number" && rule.maximum !== undefined && item > rule.maximum) throw new Error("ai.tool_arguments_invalid")
    if (rule.enum && !rule.enum.includes(item as never)) throw new Error("ai.tool_arguments_invalid")
  }
  return value
}

function matches(type: string, value: unknown): boolean {
  if (type === "array") return Array.isArray(value)
  if (type === "object") return Boolean(value) && typeof value === "object" && !Array.isArray(value)
  if (type === "integer") return typeof value === "number" && Number.isInteger(value)
  return typeof value === type
}
