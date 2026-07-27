export const HTTP_METHODS = [
  "get",
  "put",
  "post",
  "delete",
  "options",
  "head",
  "patch",
  "trace",
] as const;

export type HttpMethod = (typeof HTTP_METHODS)[number];

export type OpenApiParameterLocation = "query" | "header" | "path" | "cookie";

export interface SchemaReferenceSummary {
  readonly ref?: string;
  readonly type?: string | readonly string[];
  readonly format?: string;
  readonly title?: string;
  readonly description?: string;
  readonly enum?: readonly unknown[];
  readonly const?: unknown;
  readonly default?: unknown;
  readonly nullable?: boolean;
  readonly readOnly?: boolean;
  readonly writeOnly?: boolean;
  readonly required?: readonly string[];
  readonly properties?: Readonly<Record<string, SchemaReferenceSummary>>;
  readonly items?: SchemaReferenceSummary;
  readonly additionalProperties?: boolean | SchemaReferenceSummary;
  readonly allOf?: readonly SchemaReferenceSummary[];
  readonly anyOf?: readonly SchemaReferenceSummary[];
  readonly oneOf?: readonly SchemaReferenceSummary[];
  readonly not?: SchemaReferenceSummary;
  readonly circular?: boolean;
  readonly truncated?: boolean;
  readonly allowed?: boolean;
  readonly minLength?: number;
  readonly maxLength?: number;
  readonly pattern?: string;
  readonly minimum?: number;
  readonly maximum?: number;
  readonly minItems?: number;
  readonly maxItems?: number;
  readonly uniqueItems?: boolean;
}

export interface OpenApiParameterSnapshot {
  readonly name?: string;
  readonly in?: OpenApiParameterLocation;
  readonly required?: boolean;
  readonly description?: string;
  readonly ref?: string;
  readonly schema?: SchemaReferenceSummary;
}

export interface OpenApiRequestBodySnapshot {
  readonly required: boolean;
  readonly contentTypes: readonly string[];
  readonly schemaRefs: readonly string[];
}

export interface OpenApiResponseSnapshot {
  readonly status: string;
  readonly description?: string;
  readonly contentTypes: readonly string[];
  readonly schemaRefs: readonly string[];
}

export type OpenApiSecurityRequirement = Readonly<
  Record<string, readonly string[]>
>;

export type CommandClassification =
  | "business-command"
  | "protocol-adapter"
  | "client-entry"
  | "server-entry"
  | "internal-observability"
  | "unclassified";

export type CommandRisk = "low" | "medium" | "high" | "critical";

export type CommandTransport =
  | "http"
  | "sse"
  | "websocket"
  | "download"
  | "upload";

export interface LunaCliCommandExtension {
  readonly category?: string;
  readonly tool?: string;
  readonly path?: string;
}

export type ProjectContextMode = "required" | "optional" | "none";

export interface LunaCliProjectContextExtension {
  readonly mode: ProjectContextMode;
}

/**
 * Supported fields from the OpenAPI `x-luna-cli` extension.
 *
 * The extension is intentionally permissive while the server contract is being
 * completed. Known fields are strongly typed and unknown fields remain in the
 * generated OpenAPI source instead of leaking into the runtime command model.
 */
export interface LunaCliExtensionSnapshot {
  readonly command?: string | LunaCliCommandExtension;
  readonly categoryAliases?: readonly string[];
  readonly aliases?: readonly string[];
  readonly classification?: CommandClassification;
  readonly risk?: CommandRisk;
  readonly transport?: CommandTransport;
  readonly requiredScopes?: readonly string[];
  readonly mfaPurpose?: string;
  readonly projectContext?:
    | ProjectContextMode
    | LunaCliProjectContextExtension;
  readonly streaming?: boolean;
  readonly hidden?: boolean;
  readonly agentAllowed?: boolean;
  readonly examples?: readonly string[];
  readonly exclusionReason?: string;
}

export interface OpenApiOperationSnapshot {
  readonly method: HttpMethod;
  readonly path: string;
  readonly tags: readonly string[];
  readonly summary?: string;
  readonly description?: string;
  readonly operationId?: string;
  readonly deprecated: boolean;
  readonly security: readonly OpenApiSecurityRequirement[];
  readonly parameters: readonly OpenApiParameterSnapshot[];
  readonly requestBody?: OpenApiRequestBodySnapshot;
  readonly inputSchema?: SchemaReferenceSummary;
  readonly responses: readonly OpenApiResponseSnapshot[];
  readonly xLunaCli?: LunaCliExtensionSnapshot;
}

export type MetadataSource = "explicit" | "fallback";

export interface CommandMetadata {
  readonly canonicalPath: string;
  readonly category: string;
  readonly tool: string;
  readonly categoryAliases: readonly string[];
  readonly aliases: readonly string[];
  readonly source: MetadataSource;
  readonly classification: CommandClassification;
  readonly risk: CommandRisk;
  readonly transport: CommandTransport;
  readonly requiredScopes: readonly string[];
  readonly mfaPurpose?: string;
  readonly projectContext: ProjectContextMode;
  readonly streaming: boolean;
  readonly hidden: boolean;
  readonly agentAllowed: boolean;
  readonly examples: readonly string[];
  readonly exclusionReason?: string;
}

export interface OperationCatalogEntry extends OpenApiOperationSnapshot {
  readonly operationKey: string;
  readonly normalizedPath: string;
  readonly primaryTag: string;
  readonly operationId: string;
  readonly operationIdSource: MetadataSource;
  readonly explicitOperationId?: string;
  readonly fallbackOperationId: string;
  readonly command: CommandMetadata;
}

export interface OpenApiSnapshotMetadata {
  readonly source: string;
  readonly openapiVersion: string;
  readonly apiVersion: string;
  readonly sourceDigest: `sha256:${string}`;
  readonly operationCount: number;
}

export interface OperationCatalogMetadata {
  readonly catalogVersion: string;
  readonly apiVersion: string;
  readonly openapiVersion: string;
  readonly openapiDigest: `sha256:${string}`;
  readonly catalogDigest: `sha256:${string}`;
  readonly operationCount: number;
  readonly explicitOperationIdCount: number;
  readonly fallbackOperationIdCount: number;
  readonly explicitCommandCount: number;
  readonly fallbackCommandCount: number;
}

export interface OperationCatalogFilter {
  readonly query?: string;
  readonly method?: HttpMethod | readonly HttpMethod[];
  readonly tag?: string | readonly string[];
  readonly category?: string | readonly string[];
  readonly classification?: CommandClassification | readonly CommandClassification[];
  readonly risk?: CommandRisk | readonly CommandRisk[];
  readonly transport?: CommandTransport | readonly CommandTransport[];
  readonly scope?: string | readonly string[];
  readonly operationIdSource?: MetadataSource;
  readonly commandSource?: MetadataSource;
  readonly includeDeprecated?: boolean;
  readonly includeHidden?: boolean;
}

export interface OperationCatalogPage {
  readonly items: readonly OperationCatalogEntry[];
  readonly total: number;
  readonly offset: number;
  readonly limit: number;
  readonly nextOffset?: number;
}
