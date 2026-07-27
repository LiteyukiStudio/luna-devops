import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import {
  mkdtempSync,
  readFileSync,
  rmSync,
  statSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const httpMethods = new Set([
  "delete",
  "get",
  "head",
  "options",
  "patch",
  "post",
  "put",
  "trace",
]);
const allowedNonCommandClassifications = new Set([
  "protocol-adapter",
  "browser-callback",
  "webhook-receiver",
]);
const bypassableRiskPolicyErrors = new Set([
  "confirmation_required",
  "operation_cancelled",
]);
const riskOrder = Object.freeze(["low", "medium", "high", "critical"]);

/**
 * Non-command routes are audited one by one. Keep this list exact: prefix,
 * domain, and wildcard exclusions would hide newly added business APIs.
 */
export const NON_COMMAND_ROUTE_CLASSIFICATIONS = Object.freeze({
  "GET /api/v1/auth/oidc/{providerId}/start": Object.freeze({
    classification: "browser-callback",
    reason: "Starts an interactive browser redirect to an external OIDC provider.",
  }),
  "GET /api/v1/auth/oidc/callback": Object.freeze({
    classification: "browser-callback",
    reason: "Receives the browser redirect from an external OIDC provider.",
  }),
  "GET /api/v1/git/providers/{providerId}/oauth/start": Object.freeze({
    classification: "browser-callback",
    reason: "Starts an interactive browser redirect to a Git OAuth provider.",
  }),
  "GET /api/v1/git/oauth/callback": Object.freeze({
    classification: "browser-callback",
    reason: "Receives the browser redirect from a Git OAuth provider.",
  }),
  "GET /api/v1/oauth/authorize": Object.freeze({
    classification: "browser-callback",
    reason: "Renders the interactive OAuth authorization decision flow.",
  }),
  "POST /api/v1/oauth/authorize": Object.freeze({
    classification: "browser-callback",
    reason: "Submits the interactive OAuth authorization decision.",
  }),
  "GET /api/v1/oauth/device/verification": Object.freeze({
    classification: "browser-callback",
    reason: "Renders the browser verification page for the OAuth Device Code flow.",
  }),
  "POST /api/v1/oauth/device/verification": Object.freeze({
    classification: "browser-callback",
    reason: "Submits the browser verification decision for the OAuth Device Code flow.",
  }),
  "POST /api/v1/oauth/device/authorization": Object.freeze({
    classification: "protocol-adapter",
    reason: "Consumed by the CLI Device Authorization Grant adapter behind `luna login`.",
  }),
  "POST /api/v1/oauth/token": Object.freeze({
    classification: "protocol-adapter",
    reason: "Consumed by OAuth grant adapters and not exposed as a raw business command.",
  }),
  "POST /api/v1/oauth/revoke": Object.freeze({
    classification: "protocol-adapter",
    reason: "Consumed by the CLI logout adapter and not exposed as a raw business command.",
  }),
  "POST /api/v1/runtime/clusters/{clusterId}/pods/terminal/authorize": Object.freeze({
    classification: "protocol-adapter",
    reason: "Issues the short-lived authorization consumed by the cluster terminal adapter.",
  }),
  "GET /api/v1/runtime/clusters/{clusterId}/pods/terminal": Object.freeze({
    classification: "protocol-adapter",
    reason: "WebSocket transport consumed by the cluster terminal command adapter.",
  }),
  "GET /api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/metrics/stream": Object.freeze({
    classification: "protocol-adapter",
    reason: "Streaming transport consumed by the deployment metrics command adapter.",
  }),
  "POST /api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/data-export/authorize": Object.freeze({
    classification: "protocol-adapter",
    reason: "Issues the short-lived authorization consumed by the data export adapter.",
  }),
  "GET /api/v1/projects/{projectId}/applications/{applicationId}/deployment-targets/{targetId}/data-export": Object.freeze({
    classification: "protocol-adapter",
    reason: "Download transport consumed by the deployment data export command adapter.",
  }),
  "GET /api/v1/projects/{projectId}/build-jobs/{jobId}/logs/stream": Object.freeze({
    classification: "protocol-adapter",
    reason: "Streaming transport consumed by the build log command adapter.",
  }),
  "POST /api/v1/projects/{projectId}/releases/{releaseId}/terminal/authorize": Object.freeze({
    classification: "protocol-adapter",
    reason: "Issues the short-lived authorization consumed by the release terminal adapter.",
  }),
  "GET /api/v1/projects/{projectId}/releases/{releaseId}/terminal": Object.freeze({
    classification: "protocol-adapter",
    reason: "WebSocket transport consumed by the release terminal command adapter.",
  }),
  "POST /api/v1/git/webhooks/{bindingId}": Object.freeze({
    classification: "webhook-receiver",
    reason: "Receives signed events from an external Git provider.",
  }),
  "POST /api/v1/billing/gateway-traffic/hello": Object.freeze({
    classification: "webhook-receiver",
    reason: "Receives authenticated lifecycle reports from the gateway traffic probe.",
  }),
  "POST /api/v1/billing/gateway-traffic": Object.freeze({
    classification: "webhook-receiver",
    reason: "Receives authenticated usage windows from the gateway traffic probe.",
  }),
});

const tagCategoryOverrides = Object.freeze({
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

function splitWords(value) {
  return String(value)
    .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
    .replace(/([A-Z]+)([A-Z][a-z])/g, "$1 $2")
    .split(/[^A-Za-z0-9]+/)
    .filter(Boolean)
    .map(part => part.toLowerCase());
}

function toKebabCase(value) {
  return splitWords(value).join("-");
}

export function normalizeRoutePath(path) {
  const withLeadingSlash = path.startsWith("/") ? path : `/${path}`;
  const normalized = withLeadingSlash
    .replace(/\/:([A-Za-z][A-Za-z0-9_]*)/g, "/{$1}")
    .replace(/\/+/g, "/");
  return normalized.length > 1 ? normalized.replace(/\/+$/, "") : normalized;
}

function routeKey(method, path) {
  return `${method.toUpperCase()} ${normalizeRoutePath(path)}`;
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Extracts routes registered directly on a Gin group whose base path is
 * `/api/v1`. The source location is retained for actionable gate failures.
 */
export function extractGinRoutes(source, sourceName = "router.go") {
  const groupNames = new Set();
  const groupPattern =
    /\b([A-Za-z_][A-Za-z0-9_]*)\s*:?=\s*[A-Za-z_][A-Za-z0-9_]*\.Group\(\s*["`]\/api\/v1["`]\s*\)/g;
  for (const match of source.matchAll(groupPattern)) {
    groupNames.add(match[1]);
  }
  if (groupNames.size === 0) {
    throw new Error(`${sourceName}: no Gin group registered for /api/v1`);
  }

  const routes = [];
  const lines = source.split(/\r?\n/);
  for (const groupName of groupNames) {
    const routePattern = new RegExp(
      `^\\s*${escapeRegExp(groupName)}\\.(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\\(\\s*["\`]([^"\`]+)["\`]`,
    );
    for (let index = 0; index < lines.length; index += 1) {
      const match = lines[index].match(routePattern);
      if (!match) {
        continue;
      }
      const method = match[1].toUpperCase();
      const path = normalizeRoutePath(`/api/v1${match[2]}`);
      routes.push(Object.freeze({
        key: routeKey(method, path),
        method,
        path,
        domain: path.split("/").filter(Boolean)[2] ?? "root",
        source: sourceName,
        line: index + 1,
      }));
    }
  }
  return routes;
}

function loadYaml() {
  const requireFromContract = createRequire(
    join(repositoryRoot, "packages", "api-contract", "package.json"),
  );
  try {
    return requireFromContract("yaml");
  } catch (error) {
    throw new Error(
      `Unable to load the workspace YAML parser. Run pnpm install first: ${error.message}`,
    );
  }
}

function parseExplicitCommand(extension) {
  const command = extension?.command;
  if (!command) {
    return undefined;
  }
  let category;
  let tool;
  if (typeof command === "string") {
    const separator = command.indexOf(".");
    if (separator > 0 && separator < command.length - 1) {
      category = command.slice(0, separator);
      tool = command.slice(separator + 1);
    }
  } else {
    const explicitPath = command.path;
    if (typeof explicitPath === "string") {
      const separator = explicitPath.indexOf(".");
      if (separator > 0 && separator < explicitPath.length - 1) {
        category = explicitPath.slice(0, separator);
        tool = explicitPath.slice(separator + 1);
      }
    }
    category ??= command.category;
    tool ??= command.tool;
  }
  const normalizedCategory = toKebabCase(category ?? "");
  const normalizedTool = toKebabCase(tool ?? "");
  return normalizedCategory && normalizedTool
    ? `${normalizedCategory}.${normalizedTool}`
    : undefined;
}

function fallbackCommandPath(tag, method, path) {
  const normalizedTag = toKebabCase(tag || "Api") || "api";
  const category =
    tagCategoryOverrides[normalizedTag.replaceAll("-", "")] ?? normalizedTag;
  const segments = normalizeRoutePath(path)
    .split("/")
    .filter(Boolean)
    .slice(2)
    .map((segment) => {
      const parameter = segment.match(/^\{(.+)\}$/);
      return parameter
        ? `by-${toKebabCase(parameter[1] ?? "parameter")}`
        : toKebabCase(segment);
    })
    .filter(Boolean);
  return `${category}.${method.toLowerCase()}-${segments.join("-") || "root"}`;
}

export function parseOpenApiSource(source, sourceName = "openapi.yaml") {
  const document = loadYaml().parse(source);
  const operations = [];
  for (const [path, pathItem] of Object.entries(document?.paths ?? {})) {
    for (const [method, operation] of Object.entries(pathItem ?? {})) {
      if (!httpMethods.has(method.toLowerCase()) || !operation) {
        continue;
      }
      const normalizedPath = normalizeRoutePath(path);
      const extension = operation["x-luna-cli"] ?? {};
      const commandPath =
        parseExplicitCommand(extension) ??
        fallbackCommandPath(operation.tags?.[0] ?? "Api", method, normalizedPath);
      operations.push(Object.freeze({
        key: routeKey(method, normalizedPath),
        method: method.toUpperCase(),
        path: normalizedPath,
        commandPath,
        classification: extension.classification ?? "unclassified",
        risk: extension.risk ?? "low",
        hidden: extension.hidden === true,
        exclusionReason: extension.exclusionReason?.trim() ?? "",
        source: sourceName,
      }));
    }
  }
  return operations;
}

function uniqueByKey(items, label, errors) {
  const result = new Map();
  for (const item of items) {
    if (result.has(item.key)) {
      errors.push(`duplicate ${label}: ${item.key}`);
      continue;
    }
    result.set(item.key, item);
  }
  return result;
}

function classificationEntries(classifications) {
  return classifications instanceof Map
    ? [...classifications.entries()]
    : Object.entries(classifications);
}

function normalizeCliCommand(command) {
  if (typeof command === "string") {
    return Object.freeze({
      path: command,
      source: undefined,
      hidden: false,
      risk: "low",
      serverSupported: undefined,
    });
  }
  return Object.freeze({
    path: command?.path,
    source: command?.source,
    hidden: command?.hidden === true,
    risk: command?.risk ?? "low",
    serverSupported: command?.serverSupported,
  });
}

function extractFunctionBody(source, functionName, sourceName) {
  const signature = new RegExp(
    `\\b(?:async\\s+)?function\\s+${escapeRegExp(functionName)}\\s*\\(`,
  );
  const match = signature.exec(source);
  if (!match) {
    throw new Error(`${sourceName}: function ${functionName} was not found`);
  }
  const openingBrace = source.indexOf("{", match.index + match[0].length);
  if (openingBrace < 0) {
    throw new Error(`${sourceName}: function ${functionName} has no body`);
  }

  let depth = 0;
  let quote = "";
  let escaped = false;
  let lineComment = false;
  let blockComment = false;
  for (let index = openingBrace; index < source.length; index += 1) {
    const character = source[index];
    const next = source[index + 1];

    if (lineComment) {
      if (character === "\n") {
        lineComment = false;
      }
      continue;
    }
    if (blockComment) {
      if (character === "*" && next === "/") {
        blockComment = false;
        index += 1;
      }
      continue;
    }
    if (quote) {
      if (escaped) {
        escaped = false;
      } else if (character === "\\") {
        escaped = true;
      } else if (character === quote) {
        quote = "";
      }
      continue;
    }
    if (character === "/" && next === "/") {
      lineComment = true;
      index += 1;
      continue;
    }
    if (character === "/" && next === "*") {
      blockComment = true;
      index += 1;
      continue;
    }
    if (character === "'" || character === '"' || character === "`") {
      quote = character;
      continue;
    }
    if (character === "{") {
      depth += 1;
    } else if (character === "}") {
      depth -= 1;
      if (depth === 0) {
        return source.slice(openingBrace + 1, index);
      }
    }
  }
  throw new Error(`${sourceName}: function ${functionName} has an unterminated body`);
}

/**
 * Audits the shared command execution policy for errors that reject an entire
 * risk class regardless of confirmation, MFA, or server support. Such a
 * command is registered but not executable, so it cannot satisfy coverage.
 */
export function inspectCliExecutionPolicy(
  source,
  sourceName = "cli/src/commands/executor.ts",
) {
  const body = extractFunctionBody(source, "enforceRiskPolicy", sourceName);
  const errorPattern = /new\s+CliCommandError\(\s*["']([^"']+)["']/g;
  const hardBlockers = [];
  const blockedRisks = new Set();

  for (const match of body.matchAll(errorPattern)) {
    const code = match[1];
    if (bypassableRiskPolicyErrors.has(code)) {
      continue;
    }
    const risks = code === "server_plan_required"
      ? ["high", "critical"]
      : riskOrder.slice(1);
    risks.forEach(risk => blockedRisks.add(risk));
    hardBlockers.push(Object.freeze({
      code,
      risks: Object.freeze(risks),
      source: sourceName,
    }));
  }

  return Object.freeze({
    hardBlockers: Object.freeze(hardBlockers),
    blockedRisks: Object.freeze([...blockedRisks]),
  });
}

export function evaluateCoverage({
  routes,
  openApiOperations,
  cliCommands,
  classifications = NON_COMMAND_ROUTE_CLASSIFICATIONS,
  requireAllClassificationEntries = true,
  executionPolicy = Object.freeze({
    hardBlockers: Object.freeze([]),
    blockedRisks: Object.freeze([]),
  }),
}) {
  const errors = [];
  const routeMap = uniqueByKey(routes, "platform route", errors);
  const openApiMap = uniqueByKey(openApiOperations, "OpenAPI operation", errors);
  const cliCommandMap = uniqueByKey(
    [...cliCommands]
      .map(normalizeCliCommand)
      .filter((command) => {
        if (command.path) {
          return true;
        }
        errors.push("CLI catalog contains a command without a canonical path");
        return false;
      })
      .map(command => ({ ...command, key: command.path })),
    "CLI command",
    errors,
  );
  const auditMap = new Map(classificationEntries(classifications));
  const blockedRisks = new Set(executionPolicy.blockedRisks ?? []);

  for (const blocker of executionPolicy.hardBlockers ?? []) {
    errors.push(
      `${blocker.source}: global CLI policy "${blocker.code}" hard-blocks ${blocker.risks.join("/")} commands`,
    );
  }

  for (const [key, audit] of auditMap) {
    if (!allowedNonCommandClassifications.has(audit?.classification)) {
      errors.push(`${key}: invalid non-command classification "${audit?.classification ?? ""}"`);
    }
    if (!audit?.reason?.trim()) {
      errors.push(`${key}: non-command classification must include an audit reason`);
    }
    if (requireAllClassificationEntries && !routeMap.has(key)) {
      errors.push(`${key}: audited non-command route is not registered by the platform`);
    }
    const operation = openApiMap.get(key);
    if (!operation) {
      errors.push(`${key}: audited non-command route is missing from OpenAPI`);
    } else {
      if (!operation.hidden || !operation.exclusionReason) {
        errors.push(
          `${key}: audited non-command route must be hidden and include an OpenAPI exclusion reason`,
        );
      }
      // The exact route audit is the canonical semantic classification. The
      // OpenAPI extension only has to make the exclusion explicit and hidden;
      // older contracts may use the broader protocol-adapter label.
    }
  }

  const rows = [];
  for (const route of routes) {
    const operation = openApiMap.get(route.key);
    const audit = auditMap.get(route.key);
    let classification;
    let covered = false;
    let registered = false;
    let executable = false;

    if (audit) {
      classification = audit.classification;
      covered =
        allowedNonCommandClassifications.has(classification) &&
        Boolean(operation?.hidden && operation.exclusionReason);
    } else if (!operation) {
      classification = "unclassified";
      errors.push(`${route.key}: business route is missing from OpenAPI`);
    } else if (
      operation.hidden ||
      operation.classification === "protocol-adapter" ||
      operation.classification === "client-entry" ||
      operation.classification === "server-entry" ||
      operation.classification === "internal-observability"
    ) {
      classification = "unclassified";
      errors.push(
        `${route.key}: OpenAPI marks this as "${operation.classification}" but no exact audited classification exists`,
      );
    } else {
      const command = cliCommandMap.get(operation.commandPath);
      classification = "command";
      if (!command) {
        errors.push(
          `${route.key}: generated CLI catalog is missing command "${operation.commandPath}"`,
        );
      } else if (command.hidden) {
        errors.push(
          `${route.key}: command "${operation.commandPath}" is hidden and cannot satisfy business coverage`,
        );
      } else if (command.source && command.source !== "openapi") {
        errors.push(
          `${route.key}: command "${operation.commandPath}" is registered from "${command.source}", expected "openapi"`,
        );
      } else {
        registered = true;
        if (command.serverSupported === false) {
          errors.push(
            `${route.key}: command "${operation.commandPath}" is registered but marked unsupported by the server`,
          );
        } else if (blockedRisks.has(command.risk ?? operation.risk)) {
          errors.push(
            `${route.key}: command "${operation.commandPath}" is registered but globally blocked at risk "${command.risk ?? operation.risk}"`,
          );
        } else {
          executable = true;
          covered = true;
        }
      }
    }

    rows.push(Object.freeze({
      ...route,
      classification,
      covered,
      openapi: Boolean(operation),
      registered,
      executable,
      cli: executable,
      commandPath: operation?.commandPath,
      reason: audit?.reason,
    }));
  }

  const domains = new Map();
  for (const row of rows) {
    const stats = domains.get(row.domain) ?? {
      domain: row.domain,
      platform: 0,
      openapi: 0,
      registered: 0,
      executable: 0,
      excluded: 0,
      covered: 0,
    };
    stats.platform += 1;
    stats.openapi += row.openapi ? 1 : 0;
    stats.registered += row.registered ? 1 : 0;
    stats.executable += row.executable ? 1 : 0;
    stats.excluded += allowedNonCommandClassifications.has(row.classification) ? 1 : 0;
    stats.covered += row.covered ? 1 : 0;
    domains.set(row.domain, stats);
  }

  const domainStats = [...domains.values()]
    .sort((left, right) => left.domain.localeCompare(right.domain))
    .map(stats => Object.freeze({
      ...stats,
      coverage: stats.platform === 0 ? 100 : (stats.covered / stats.platform) * 100,
    }));
  const totals = domainStats.reduce(
    (sum, stats) => {
      for (const field of [
        "platform",
        "openapi",
        "registered",
        "executable",
        "excluded",
        "covered",
      ]) {
        sum[field] += stats[field];
      }
      return sum;
    },
    {
      platform: 0,
      openapi: 0,
      registered: 0,
      executable: 0,
      excluded: 0,
      covered: 0,
    },
  );
  totals.cli = totals.executable;
  totals.coverage =
    totals.platform === 0 ? 100 : (totals.covered / totals.platform) * 100;
  totals.businessRoutes = totals.platform - totals.excluded;
  totals.registeredBusinessCoverage =
    totals.businessRoutes === 0
      ? 100
      : (totals.registered / totals.businessRoutes) * 100;
  totals.businessCoverage =
    totals.businessRoutes === 0
      ? 100
      : (totals.executable / totals.businessRoutes) * 100;

  if (totals.businessCoverage !== 100) {
    errors.push(
      `business command coverage is ${totals.businessCoverage.toFixed(1)}%, expected 100.0%`,
    );
  }

  return Object.freeze({
    ok: errors.length === 0,
    errors: Object.freeze(errors),
    rows: Object.freeze(rows),
    domains: Object.freeze(domainStats),
    totals: Object.freeze(totals),
  });
}

function cliInvocation() {
  const tsx = join(repositoryRoot, "cli", "node_modules", ".bin", "tsx");
  try {
    if (statSync(tsx).isFile()) {
      return {
        command: tsx,
        prefix: [join(repositoryRoot, "cli", "src", "entry.ts")],
      };
    }
  } catch {
    // Release or minimal checkouts may only contain the built CLI.
  }
  return {
    command: process.execPath,
    prefix: [join(repositoryRoot, "cli", "dist", "entry.js")],
  };
}

function loadCliCatalog() {
  const lunaHome = mkdtempSync(join(tmpdir(), "luna-platform-coverage-"));
  const items = [];
  const seenCursors = new Set();
  let cursor;
  try {
    do {
      const runner = cliInvocation();
      const args = [
        ...runner.prefix,
        "help",
        "catalog",
        // Hidden/raw commands are deliberately omitted: they are transport
        // details and cannot satisfy visible business-command coverage.
        "limit=100",
        ...(cursor ? [`cursor=${cursor}`] : []),
        "agent=true",
      ];
      const result = spawnSync(runner.command, args, {
        cwd: repositoryRoot,
        encoding: "utf8",
        env: {
          ...process.env,
          LUNA_HOME: lunaHome,
          LUNA_LANG: "en-US",
        },
        timeout: 120_000,
      });
      if (result.error) {
        throw result.error;
      }
      if (result.status !== 0) {
        throw new Error(
          result.stderr || result.stdout || "unable to read Luna command catalog",
        );
      }
      const envelope = JSON.parse(result.stdout);
      items.push(...(envelope?.data?.items ?? []));
      cursor = envelope?.data?.nextCursor;
      if (cursor) {
        if (seenCursors.has(cursor)) {
          throw new Error(`CLI catalog returned a repeated cursor: ${cursor}`);
        }
        seenCursors.add(cursor);
      }
    } while (cursor);
    return items;
  } finally {
    rmSync(lunaHome, { recursive: true, force: true });
  }
}

export function formatCoverageReport(result) {
  const header = [
    "domain",
    "platform",
    "openapi",
    "registered",
    "executable",
    "excluded",
    "coverage",
  ];
  const records = result.domains.map(stats => [
    stats.domain,
    String(stats.platform),
    String(stats.openapi),
    String(stats.registered),
    String(stats.executable),
    String(stats.excluded),
    `${stats.coverage.toFixed(1)}%`,
  ]);
  records.push([
    "TOTAL",
    String(result.totals.platform),
    String(result.totals.openapi),
    String(result.totals.registered),
    String(result.totals.executable),
    String(result.totals.excluded),
    `${result.totals.coverage.toFixed(1)}%`,
  ]);
  const widths = header.map((value, index) =>
    Math.max(value.length, ...records.map(record => record[index].length)),
  );
  const render = record =>
    record.map((value, index) => value.padEnd(widths[index])).join("  ");
  const output = [
    "Platform route vs OpenAPI/CLI coverage",
    render(header),
    render(widths.map(width => "-".repeat(width))),
    ...records.map(render),
    "",
    `Registered business commands: ${result.totals.registered}/${result.totals.businessRoutes} (${result.totals.registeredBusinessCoverage.toFixed(1)}%; target 100.0%)`,
    `Executable business commands: ${result.totals.executable}/${result.totals.businessRoutes} (${result.totals.businessCoverage.toFixed(1)}%; target 100.0%)`,
  ];
  if (result.errors.length > 0) {
    output.push("", `Failures (${result.errors.length}):`);
    output.push(...result.errors.map(error => `- ${error}`));
  }
  return output.join("\n");
}

export async function runPlatformCliCoverageGate() {
  const routerPath = join(repositoryRoot, "internal", "api", "router.go");
  const openApiPath = join(repositoryRoot, "openapi", "openapi.yaml");
  const executorPath = join(
    repositoryRoot,
    "cli",
    "src",
    "commands",
    "executor.ts",
  );
  const routes = extractGinRoutes(
    readFileSync(routerPath, "utf8"),
    relative(repositoryRoot, routerPath),
  );
  const openApiOperations = parseOpenApiSource(
    readFileSync(openApiPath, "utf8"),
    relative(repositoryRoot, openApiPath),
  );
  const executionPolicy = inspectCliExecutionPolicy(
    readFileSync(executorPath, "utf8"),
    relative(repositoryRoot, executorPath),
  );
  const cliCommands = loadCliCatalog();
  const result = evaluateCoverage({
    routes,
    openApiOperations,
    cliCommands,
    executionPolicy,
  });
  process.stdout.write(`${formatCoverageReport(result)}\n`);
  if (!result.ok) {
    process.exitCode = 1;
  }
  return result;
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  runPlatformCliCoverageGate().catch((error) => {
    process.stderr.write(
      `Platform route coverage check failed: ${error instanceof Error ? error.message : error}\n`,
    );
    process.exitCode = 1;
  });
}
