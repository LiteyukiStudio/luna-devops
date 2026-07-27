# Source Development

Luna CLI lives in the `cli/` workspace and reuses `packages/api-contract` and
`packages/api-client`. OpenAPI is the sole source of truth for ordinary business
commands, so the CLI does not maintain a second handwritten API inventory.

## Run from source

Install the locked workspace dependencies from the repository root:

```bash
pnpm install --frozen-lockfile
```

Use a temporary home directory so development never reads real credentials:

```bash
export LUNA_HOME="$(mktemp -d)"
pnpm --silent --dir cli exec tsx src/entry.ts version show
pnpm --silent --dir cli exec tsx src/entry.ts help catalog query=project limit=10 output=json interactive=false
```

The normal location is `~/.luna`. Development, tests, and CI must use a temporary `LUNA_HOME`.

## Workspace responsibilities

| Path | Responsibility |
| --- | --- |
| `cli/src/commands` | Command registration, parsing, risk gates, and execution |
| `cli/src/auth` | Access Tokens, local credentials, and authentication status |
| `cli/src/config` | Active server, credential, and default project |
| `cli/src/input` | `key=value`, JSON, files, and standard input |
| `cli/src/output` | Human output, JSON envelopes, and redaction |
| `packages/api-contract` | Environment-neutral contracts generated from OpenAPI |
| `packages/api-client` | HTTP client shared by Web and CLI |
| `scripts/cli` | Contract checks, packaging, artifact verification, and release tools |
| `ai-supports/skills` | Agent Skills that operate through the CLI only |

## Command sources and boundaries

Commands have exactly three sources:

1. **OpenAPI business commands**: public JSON HTTP control-plane APIs enter
   OpenAPI first with a stable `operationId`, Scope, input/output schemas, and
   `x-luna-cli` metadata.
2. **Protocol adapters**: only for SSE, WebSocket, file download, and
   authorized follow-up transports that a normal JSON HTTP command cannot
   represent correctly.
3. **Local commands**: login, local configuration, help, and completion
   capabilities that do not map to a platform business API.

Generated business operations are registered as:

```text
luna <category> <tool> key=value
```

Purely local capabilities such as authentication, projects, help, and
completion are registered in `cli/src/commands/local.ts`. Do not create a
parallel implementation for an existing ordinary HTTP API.

A protocol adapter must:

- target a real platform route and never call Kubernetes, Git, or registry providers directly;
- implement only streaming, bidirectional, or binary transport instead of duplicating an ordinary OpenAPI command;
- mark its OpenAPI operation as a hidden protocol operation with an exclusion reason;
- register and validate the exact `method + path` in the platform coverage gate.

Browser callbacks, webhook receivers, and pre-application bootstrap endpoints
may be excluded from CLI commands, but every route still requires an exact
classification and reason. Directory, prefix, and domain wildcards are not
valid exclusions.

After changing OpenAPI, run:

```bash
pnpm --filter @luna-devops/api-contract generate
node scripts/cli/verify-contract-drift.mjs
```

## Machine output and Skills

AI agents must explicitly set
`output=json interactive=false agent=true` on every command instead of relying
on local output or interaction defaults. Agent mode also disables colors and
requires canonical command names:

```bash
luna help catalog all=true limit=100 output=json interactive=false agent=true
luna help command path=registry.get-registries output=json interactive=false agent=true
```

Agents parse JSON from `stdout` only and treat `stderr` as diagnostics. They do
not parse human tables, prose help, or terminal colors. Machine-readable Help
is the source of truth for commands, parameters, risks, and output contracts.
`ai-supports/skills` describes intent routing, workflow, and safety boundaries
without copying the command manual.

After changing commands, parameters, risks, capability boundaries, or Skills,
run:

```bash
pnpm check:cli-skills
```

The check rejects unknown literal commands, Agent commands that do not fix
`output=json interactive=false agent=true`, and known stale capability claims.

After `project use` defines a default project, the executor injects its immutable ID into
required `project`, `projectId`, or `projectID` parameters. Explicit command values
take precedence. Optional project parameters are not injected, which keeps global and
cross-project requests free from accidental filters.

`api request` is reserved for human diagnostics against a known relative API path.
Agent mode always rejects it and exposes no configuration or parameter bypass.

## Coverage gate

The repository gate decides whether the CLI covers the platform's public
business surface:

```bash
pnpm check:platform-cli-coverage
```

The command:

- extracts `/api/v1` routes from the Gin Router;
- reads OpenAPI operations and CLI classifications;
- reads every page of the machine command catalog;
- compares platform routes, OpenAPI, CLI commands, and explicit exclusions by domain;
- rejects business routes without OpenAPI, OpenAPI operations without CLI
  commands, unaudited protocol/callback/webhook routes, and exclusions without
  reasons;
- requires 100% ordinary business-command coverage.

Exit status `0` is the acceptance criterion. The live totals, domain breakdown,
and ratios printed by the command are the only coverage statistics; docs and
TODO items do not duplicate them.

“Full coverage” means every public platform route is exactly one of:

1. an ordinary business API documented in OpenAPI with a canonical machine-catalog command;
2. a special transport consumed by a protocol adapter and audited by both OpenAPI metadata and exact route classification;
3. a non-CLI browser callback, webhook receiver, or explicit exclusion with a documented exact-route reason.

`api request` is a human diagnostic tool. It does not count toward coverage and
cannot replace a missing business command. Remote `high`/`critical` commands
require per-operation confirmation in an interactive terminal; non-interactive
and Agent callers must pass `--yes`. CLI confirmation records caller intent
only, and catalog coverage does not bypass backend permissions, scopes, step-up
MFA, or resource-consistency policy. Terminal and data-export protocols also
require CLI OAuth plus a valid step-up assertion for the matching purpose; a
personal access token cannot substitute for it.

## Validation

```bash
pnpm --filter @luna-devops/api-contract generate
node scripts/cli/verify-contract-drift.mjs
pnpm --filter @liteyuki/luna-cli typecheck
pnpm --filter @liteyuki/luna-cli lint
pnpm --filter @liteyuki/luna-cli test
pnpm --filter @liteyuki/luna-cli build
node --test scripts/cli/tests/*.test.mjs
pnpm check:platform-cli-coverage
pnpm check:cli-skills
```

Contract drift, platform coverage, and Skill synchronization together prevent
the platform, OpenAPI, CLI, and Agent guidance from evolving independently.
Run at least those three checks after adding or changing a public business API;
run the full block before release.

See `notes/cli-spec.md` and `TODO.md` in the repository for the full design and remaining work.
