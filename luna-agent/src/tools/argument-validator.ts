import { Ajv2020, type ErrorObject, type ValidateFunction } from "ajv/dist/2020.js"
import formatsModule from "ajv-formats"
import type { ToolArgumentIssue, ToolArgumentsInvalid } from "./contracts.js"

const ajv = new Ajv2020({
  allErrors: true,
  allowUnionTypes: true,
  strict: false,
  validateFormats: true,
})
const addFormats = formatsModule as unknown as (instance: Ajv2020) => Ajv2020
addFormats(ajv)

const validatorCache = new WeakMap<object, ValidateFunction>()

export class ToolArgumentsInvalidError extends Error implements ToolArgumentsInvalid {
  readonly code = "ai.tool_arguments_invalid" as const
  readonly retryable = true

  constructor(readonly issues: ToolArgumentIssue[]) {
    super("ai.tool_arguments_invalid")
    this.name = "ToolArgumentsInvalidError"
  }

  toJSON(): ToolArgumentsInvalid {
    return { code: this.code, retryable: this.retryable, issues: this.issues }
  }
}

export function validateToolArguments(schema: Record<string, unknown>, input: unknown): Record<string, unknown> {
  const validate = compile(schema)
  if (!validate(input))
    throw new ToolArgumentsInvalidError(toIssues(validate.errors ?? []))
  if (!input || typeof input !== "object" || Array.isArray(input))
    throw new ToolArgumentsInvalidError([{ path: "", code: "type", remediation: "请提供 JSON 对象。" }])
  return input as Record<string, unknown>
}

export function requiredInputFields(error: ToolArgumentsInvalidError): string[] | undefined {
  if (error.issues.length === 0 || error.issues.some(issue => issue.code !== "required"))
    return undefined
  return [...new Set(error.issues.map(issue => rootFieldOrPointer(issue.path)))]
}

function compile(schema: Record<string, unknown>): ValidateFunction {
  const cached = validatorCache.get(schema)
  if (cached) return cached
  const validate = ajv.compile(schema)
  validatorCache.set(schema, validate)
  return validate
}

function toIssues(errors: ErrorObject[]): ToolArgumentIssue[] {
  const issues = errors.map(toIssue)
  const unique = new Map<string, ToolArgumentIssue>()
  for (const issue of issues) {
    const key = `${issue.path}\u0000${issue.code}\u0000${JSON.stringify(issue.allowedValues ?? [])}`
    if (!unique.has(key)) unique.set(key, issue)
  }
  return [...unique.values()]
}

function toIssue(error: ErrorObject): ToolArgumentIssue {
  const path = issuePath(error)
  const allowedValues = error.keyword === "enum"
    ? cloneAllowedValues((error.params as { allowedValues?: unknown[] }).allowedValues)
    : error.keyword === "const"
      ? cloneAllowedValues([(error.params as { allowedValue?: unknown }).allowedValue])
      : undefined
  return {
    path,
    code: error.keyword,
    ...(allowedValues ? { allowedValues } : {}),
    remediation: remediation(error, path, allowedValues),
  }
}

function issuePath(error: ErrorObject): string {
  if (error.keyword === "required") {
    const missingProperty = (error.params as { missingProperty: string }).missingProperty
    return `${error.instancePath}/${escapePointerToken(missingProperty)}`
  }
  if (error.keyword === "additionalProperties") {
    const additionalProperty = (error.params as { additionalProperty: string }).additionalProperty
    return `${error.instancePath}/${escapePointerToken(additionalProperty)}`
  }
  return error.instancePath
}

function remediation(error: ErrorObject, path: string, allowedValues: unknown[] | undefined): string {
  const target = path || "根参数"
  if (error.keyword === "required") return `请补充必填字段 ${target}。`
  if (error.keyword === "additionalProperties") return `请移除 Schema 未声明的字段 ${target}。`
  if (error.keyword === "enum" || error.keyword === "const")
    return `请将 ${target} 修改为 allowedValues 中的值：${JSON.stringify(allowedValues ?? [])}。`
  if (error.keyword === "type") return `请按 Schema 要求修正 ${target} 的数据类型。`
  if (error.keyword === "format") return `请将 ${target} 修改为合法的 ${(error.params as { format?: string }).format ?? "指定"} 格式。`
  if (error.keyword === "pattern") return `请将 ${target} 修改为符合 Schema pattern 的字符串。`
  if (["minimum", "exclusiveMinimum", "maximum", "exclusiveMaximum", "multipleOf"].includes(error.keyword))
    return `请将 ${target} 修改为符合 Schema 数值范围的值。`
  if (["minLength", "maxLength"].includes(error.keyword)) return `请将 ${target} 修改为符合 Schema 长度限制的字符串。`
  if (["minItems", "maxItems", "uniqueItems", "contains"].includes(error.keyword)) return `请将 ${target} 修改为符合 Schema 数组约束的值。`
  if (["oneOf", "anyOf", "allOf", "not", "if"].includes(error.keyword)) return `请将 ${target} 修改为符合 Schema 组合约束的值。`
  return `请根据 JSON Schema 修正 ${target}。`
}

function rootFieldOrPointer(pointer: string): string {
  const tokens = pointer.split("/").slice(1)
  return tokens.length === 1 ? unescapePointerToken(tokens[0] ?? "") : pointer
}

function escapePointerToken(value: string): string {
  return value.replaceAll("~", "~0").replaceAll("/", "~1")
}

function unescapePointerToken(value: string): string {
  return value.replaceAll("~1", "/").replaceAll("~0", "~")
}

function cloneAllowedValues(values: unknown[] | undefined): unknown[] | undefined {
  return values?.map(value => structuredClone(value))
}
