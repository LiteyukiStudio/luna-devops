import { ToolCatalog, type ToolOperation } from "../../src/tools/catalog.js"

export const emptyToolInputSchema = Object.freeze({
  type: "object" as const,
  properties: {},
  required: [],
  additionalProperties: false as const,
})

export function testToolOperation(
  operationId: string,
  overrides: Partial<ToolOperation> = {},
): ToolOperation {
  return {
    operationId,
    name: operationId,
    summary: operationId,
    category: "test",
    tags: ["Test"],
    aliases: { zh: [], en: [] },
    purpose: { zh: operationId, en: operationId },
    avoidWhen: { zh: "", en: "" },
    preconditions: { zh: [], en: [] },
    successEvidence: { zh: "", en: "" },
    requiresApproval: false,
    idempotent: true,
    method: "GET",
    path: `/api/v1/test/${operationId}`,
    requiredScopes: [],
    inputSchema: emptyToolInputSchema,
    outputSchema: {},
    sensitivePaths: [],
    parameters: [],
    requestBody: false,
    requestRequired: false,
    requestType: "",
    ...overrides,
  }
}

export function testToolCatalog(...operations: ToolOperation[]): ToolCatalog {
  return ToolCatalog.load(operations.length ? operations : [testToolOperation("testOperation")])
}
