import {
  OPENAPI_OPERATION_SNAPSHOTS,
  OPENAPI_SNAPSHOT_METADATA,
} from "./generated/operations.js";
import { digestJson } from "./digest.js";
import type {
  CommandClassification,
  CommandMetadata,
  CommandRisk,
  CommandTransport,
  HttpMethod,
  MetadataSource,
  OpenApiOperationSnapshot,
  OperationCatalogEntry,
  OperationCatalogFilter,
  OperationCatalogMetadata,
  OperationCatalogPage,
  ProjectContextMode,
} from "./types.js";

const TAG_CATEGORY_OVERRIDES: Readonly<Record<string, string>> = Object.freeze({
  accesstokens: "access-token",
  applications: "application",
  builds: "build",
  configs: "config",
  dataretention: "data-retention",
  deployments: "deployment",
  projects: "project",
  registries: "registry",
  users: "user",
});

function splitWords(value: string): string[] {
  return value
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/([A-Z]+)([A-Z][a-z])/g, "$1 $2")
    .split(/[^A-Za-z0-9]+/)
    .filter(Boolean)
    .map((part) => part.toLowerCase());
}

export function toKebabCase(value: string): string {
  return splitWords(value).join("-");
}

function toSnakeCase(value: string): string {
  return splitWords(value).join("_");
}

/**
 * Normalizes Gin and OpenAPI parameter syntax to the OpenAPI form.
 */
export function normalizeOpenApiPath(path: string): string {
  const withLeadingSlash = path.startsWith("/") ? path : `/${path}`;
  const normalized = withLeadingSlash
    .replace(/\/:([A-Za-z][A-Za-z0-9_]*)/g, "/{$1}")
    .replace(/\/+/g, "/");
  return normalized.length > 1 ? normalized.replace(/\/+$/, "") : normalized;
}

export function createOperationKey(method: HttpMethod, path: string): string {
  return `${method.toUpperCase()} ${normalizeOpenApiPath(path)}`;
}

function pathSegments(path: string): string[] {
  return normalizeOpenApiPath(path)
    .split("/")
    .filter(Boolean)
    .map((segment) => {
      const parameter = segment.match(/^\{(.+)\}$/);
      return parameter
        ? `by-${toKebabCase(parameter[1] ?? "parameter")}`
        : toKebabCase(segment);
    })
    .filter(Boolean);
}

function commandPathSegments(path: string): string[] {
  const segments = pathSegments(path);
  if (segments[0] === "api" && /^v\d+$/.test(segments[1] ?? "")) {
    return segments.slice(2);
  }
  return segments;
}

function categoryFromTag(tag: string): string {
  const normalized = toKebabCase(tag) || "api";
  return TAG_CATEGORY_OVERRIDES[normalized.replaceAll("-", "")] ?? normalized;
}

export function createFallbackOperationId(
  tag: string,
  method: HttpMethod,
  path: string,
): string {
  const tagPart = toSnakeCase(tag) || "api";
  const pathPart =
    pathSegments(path).map(toSnakeCase).filter(Boolean).join("_") || "root";
  return `fallback_${tagPart}_${method}_${pathPart}`;
}

function createFallbackCommand(
  tag: string,
  method: HttpMethod,
  path: string,
): Pick<CommandMetadata, "canonicalPath" | "category" | "tool" | "source"> {
  const category = categoryFromTag(tag);
  const resourcePath = commandPathSegments(path).join("-") || "root";
  const tool = `${method}-${resourcePath}`;
  return {
    canonicalPath: `${category}.${tool}`,
    category,
    tool,
    source: "fallback",
  };
}

function parseExplicitCommand(
  operation: OpenApiOperationSnapshot,
): Pick<CommandMetadata, "canonicalPath" | "category" | "tool" | "source"> | undefined {
  const command = operation.xLunaCli?.command;
  if (!command) {
    return undefined;
  }

  let category: string | undefined;
  let tool: string | undefined;
  if (typeof command === "string") {
    const separator = command.indexOf(".");
    if (separator > 0 && separator < command.length - 1) {
      category = command.slice(0, separator);
      tool = command.slice(separator + 1);
    }
  } else {
    if (command.path) {
      const separator = command.path.indexOf(".");
      if (separator > 0 && separator < command.path.length - 1) {
        category = command.path.slice(0, separator);
        tool = command.path.slice(separator + 1);
      }
    }
    category ??= command.category;
    tool ??= command.tool;
  }

  const normalizedCategory = toKebabCase(category ?? "");
  const normalizedTool = toKebabCase(tool ?? "");
  if (!normalizedCategory || !normalizedTool) {
    return undefined;
  }

  return {
    canonicalPath: `${normalizedCategory}.${normalizedTool}`,
    category: normalizedCategory,
    tool: normalizedTool,
    source: "explicit",
  };
}

function fallbackRisk(method: HttpMethod): CommandRisk {
  switch (method) {
    case "get":
    case "head":
    case "options":
      return "low";
    case "post":
      return "medium";
    case "put":
    case "patch":
      return "high";
    case "delete":
    case "trace":
      return "critical";
  }
}

function requiredScopes(operation: OpenApiOperationSnapshot): readonly string[] {
  const extensionScopes = operation.xLunaCli?.requiredScopes;
  const scopes =
    extensionScopes ??
    operation.security.flatMap((requirement) =>
      Object.values(requirement).flatMap((values) => values),
    );
  return Object.freeze([...new Set(scopes)].sort());
}

function projectContext(
  operation: OpenApiOperationSnapshot,
): ProjectContextMode {
  const configured = operation.xLunaCli?.projectContext;
  if (typeof configured === "string") {
    return configured;
  }
  if (configured?.mode) {
    return configured.mode;
  }
  return operation.parameters.some(
    (parameter) =>
      parameter.in === "path" &&
      ["projectId", "project_id"].includes(parameter.name ?? ""),
  )
    ? "required"
    : "none";
}

function uniqueStrings(values: readonly string[] | undefined): readonly string[] {
  return Object.freeze(
    [...new Set((values ?? []).map((value) => value.trim()).filter(Boolean))],
  );
}

function commandAliases(
  operation: OpenApiOperationSnapshot,
  operationId: string,
  command: Pick<CommandMetadata, "tool">,
): readonly string[] {
  const semanticAlias = toKebabCase(operationId);
  return uniqueStrings([
    ...(operation.xLunaCli?.aliases ?? []),
    ...(semanticAlias && semanticAlias !== command.tool ? [semanticAlias] : []),
  ]);
}

function deepFreeze<T>(value: T): T {
  if (value === null || typeof value !== "object" || Object.isFrozen(value)) {
    return value;
  }

  Object.freeze(value);
  for (const nested of Object.values(value)) {
    deepFreeze(nested);
  }
  return value;
}

function catalogSort(
  left: OperationCatalogEntry,
  right: OperationCatalogEntry,
): number {
  return (
    left.command.canonicalPath.localeCompare(right.command.canonicalPath) ||
    left.operationKey.localeCompare(right.operationKey)
  );
}

export function buildOperationCatalog(
  snapshots: readonly OpenApiOperationSnapshot[],
): readonly OperationCatalogEntry[] {
  const entries = snapshots.map((operation): OperationCatalogEntry => {
    const normalizedPath = normalizeOpenApiPath(operation.path);
    const primaryTag = operation.tags[0] || "Api";
    const fallbackOperationId = createFallbackOperationId(
      primaryTag,
      operation.method,
      normalizedPath,
    );
    const explicitOperationId = operation.operationId?.trim() || undefined;
    const explicitCommand = parseExplicitCommand(operation);
    const fallbackCommand = createFallbackCommand(
      primaryTag,
      operation.method,
      normalizedPath,
    );
    const commandBase = explicitCommand ?? fallbackCommand;
    const operationId = explicitOperationId ?? fallbackOperationId;
    const command: CommandMetadata = {
      ...commandBase,
      categoryAliases: uniqueStrings(operation.xLunaCli?.categoryAliases),
      aliases: commandAliases(operation, operationId, commandBase),
      classification:
        operation.xLunaCli?.classification ?? "unclassified",
      risk: operation.xLunaCli?.risk ?? fallbackRisk(operation.method),
      transport: operation.xLunaCli?.transport ?? "http",
      requiredScopes: requiredScopes(operation),
      mfaPurpose: operation.xLunaCli?.mfaPurpose,
      projectContext: projectContext(operation),
      streaming:
        operation.xLunaCli?.streaming ??
        ["sse", "websocket"].includes(
          operation.xLunaCli?.transport ?? "http",
        ),
      hidden: operation.xLunaCli?.hidden ?? false,
      agentAllowed: operation.xLunaCli?.agentAllowed ?? true,
      examples: uniqueStrings(operation.xLunaCli?.examples),
      exclusionReason: operation.xLunaCli?.exclusionReason,
    };

    return deepFreeze({
      ...operation,
      normalizedPath,
      path: normalizedPath,
      operationKey: createOperationKey(operation.method, normalizedPath),
      primaryTag,
      operationId,
      operationIdSource: explicitOperationId ? "explicit" : "fallback",
      explicitOperationId,
      fallbackOperationId,
      command,
    });
  });

  entries.sort(catalogSort);

  const uniqueOperationKeys = new Set<string>();
  const uniqueOperationIds = new Set<string>();
  const uniqueCommandPaths = new Set<string>();
  for (const entry of entries) {
    if (uniqueOperationKeys.has(entry.operationKey)) {
      throw new Error(`Duplicate OpenAPI operation key: ${entry.operationKey}`);
    }
    if (uniqueOperationIds.has(entry.operationId)) {
      throw new Error(`Duplicate OpenAPI operation ID: ${entry.operationId}`);
    }
    if (uniqueCommandPaths.has(entry.command.canonicalPath)) {
      throw new Error(
        `Duplicate Luna command path: ${entry.command.canonicalPath}`,
      );
    }
    uniqueOperationKeys.add(entry.operationKey);
    uniqueOperationIds.add(entry.operationId);
    uniqueCommandPaths.add(entry.command.canonicalPath);
  }

  return deepFreeze(entries);
}

export const OPERATION_CATALOG = buildOperationCatalog(
  OPENAPI_OPERATION_SNAPSHOTS,
);

const OPERATIONS_BY_ID = new Map(
  OPERATION_CATALOG.map((operation) => [operation.operationId, operation]),
);
const OPERATIONS_BY_KEY = new Map(
  OPERATION_CATALOG.map((operation) => [operation.operationKey, operation]),
);
const OPERATIONS_BY_COMMAND = new Map(
  OPERATION_CATALOG.map((operation) => [
    operation.command.canonicalPath,
    operation,
  ]),
);

const catalogDigest = digestJson(OPERATION_CATALOG);
const explicitOperationIdCount = OPERATION_CATALOG.filter(
  ({ operationIdSource }) => operationIdSource === "explicit",
).length;
const explicitCommandCount = OPERATION_CATALOG.filter(
  ({ command }) => command.source === "explicit",
).length;

export const OPERATION_CATALOG_METADATA: OperationCatalogMetadata =
  deepFreeze({
    catalogVersion: `${OPENAPI_SNAPSHOT_METADATA.apiVersion}+catalog.${catalogDigest.slice(7, 19)}`,
    apiVersion: OPENAPI_SNAPSHOT_METADATA.apiVersion,
    openapiVersion: OPENAPI_SNAPSHOT_METADATA.openapiVersion,
    openapiDigest: OPENAPI_SNAPSHOT_METADATA.sourceDigest,
    catalogDigest,
    operationCount: OPERATION_CATALOG.length,
    explicitOperationIdCount,
    fallbackOperationIdCount:
      OPERATION_CATALOG.length - explicitOperationIdCount,
    explicitCommandCount,
    fallbackCommandCount: OPERATION_CATALOG.length - explicitCommandCount,
  });

export function findOperationById(
  operationId: string,
): OperationCatalogEntry | undefined {
  return OPERATIONS_BY_ID.get(operationId);
}

export function findOperationByRoute(
  method: HttpMethod,
  path: string,
): OperationCatalogEntry | undefined {
  return OPERATIONS_BY_KEY.get(createOperationKey(method, path));
}

export function findOperationByCommand(
  commandPath: string,
): OperationCatalogEntry | undefined {
  return OPERATIONS_BY_COMMAND.get(commandPath);
}

function asArray<T>(value: T | readonly T[] | undefined): readonly T[] {
  if (value === undefined) {
    return [];
  }
  return Array.isArray(value) ? (value as readonly T[]) : [value as T];
}

function includesAny<T>(
  expected: T | readonly T[] | undefined,
  actual: readonly T[],
): boolean {
  const values = asArray(expected);
  return values.length === 0 || values.some((value) => actual.includes(value));
}

function searchText(operation: OperationCatalogEntry): string {
  return [
    operation.operationId,
    operation.operationKey,
    operation.command.canonicalPath,
    operation.summary,
    operation.description,
    ...operation.tags,
    ...operation.command.requiredScopes,
  ]
    .filter(Boolean)
    .join(" ")
    .toLowerCase();
}

export function filterOperationCatalog(
  filter: OperationCatalogFilter = {},
): readonly OperationCatalogEntry[] {
  const query = filter.query?.trim().toLowerCase();
  const methods = asArray(filter.method);
  const tags = asArray(filter.tag).map((tag) => tag.toLowerCase());
  const categories = asArray(filter.category).map((category) =>
    category.toLowerCase(),
  );
  const classifications = asArray<CommandClassification>(
    filter.classification,
  );
  const risks = asArray<CommandRisk>(filter.risk);
  const transports = asArray<CommandTransport>(filter.transport);
  const scopes = asArray(filter.scope);

  return OPERATION_CATALOG.filter((operation) => {
    if (!filter.includeDeprecated && operation.deprecated) {
      return false;
    }
    if (!filter.includeHidden && operation.command.hidden) {
      return false;
    }
    if (query && !searchText(operation).includes(query)) {
      return false;
    }
    if (methods.length > 0 && !methods.includes(operation.method)) {
      return false;
    }
    if (
      tags.length > 0 &&
      !operation.tags.some((tag) => tags.includes(tag.toLowerCase()))
    ) {
      return false;
    }
    if (
      categories.length > 0 &&
      !categories.includes(operation.command.category.toLowerCase())
    ) {
      return false;
    }
    if (!includesAny(classifications, [operation.command.classification])) {
      return false;
    }
    if (!includesAny(risks, [operation.command.risk])) {
      return false;
    }
    if (!includesAny(transports, [operation.command.transport])) {
      return false;
    }
    if (
      scopes.length > 0 &&
      !scopes.every((scope) => operation.command.requiredScopes.includes(scope))
    ) {
      return false;
    }
    if (
      filter.operationIdSource &&
      filter.operationIdSource !== operation.operationIdSource
    ) {
      return false;
    }
    return !(
      filter.commandSource &&
      filter.commandSource !== operation.command.source
    );
  });
}

export function pageOperationCatalog(
  filter: OperationCatalogFilter = {},
  options: { readonly offset?: number; readonly limit?: number } = {},
): OperationCatalogPage {
  const offset = Math.max(0, Math.trunc(options.offset ?? 0));
  const limit = Math.min(500, Math.max(1, Math.trunc(options.limit ?? 50)));
  const matching = filterOperationCatalog(filter);
  const items = matching.slice(offset, offset + limit);
  const nextOffset =
    offset + items.length < matching.length ? offset + items.length : undefined;

  return {
    items,
    total: matching.length,
    offset,
    limit,
    nextOffset,
  };
}

export type {
  CommandClassification,
  CommandRisk,
  CommandTransport,
  MetadataSource,
};
