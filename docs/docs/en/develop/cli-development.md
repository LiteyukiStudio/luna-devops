# Source development

Luna CLI now lives in the standalone
[`LiteyukiStudio/luna-cli`](https://github.com/LiteyukiStudio/luna-cli)
repository. That repository is the source of truth for the CLI, API client,
OpenAPI contract snapshot, paired Skill, tests, and release automation. The
Luna DevOps repository remains the source of truth for the backend and platform
OpenAPI.

The repositories are not connected through submodules, subtree, or subrepo.
CLI CI checks out a selected platform revision read-only and verifies that
routes, OpenAPI operations, and CLI commands remain aligned.

## Local checkout

The Luna DevOps root reserves an ignored `/cli/` directory for local
integration:

```bash
cd /path/to/luna-devops
git clone git@github.com:LiteyukiStudio/luna-cli.git cli
pnpm --dir cli install --frozen-lockfile
```

The directory is not part of the platform pnpm workspace and is never included
in platform commits or release artifacts. The CLI can also be cloned anywhere
else.

## Repository layout

| CLI repository path | Responsibility |
| --- | --- |
| `src/commands` | Command registration, validation, risk gates, and execution |
| `src/auth` | Device Code, access tokens, credentials, and auth status |
| `src/config` | Active server, credentials, and default project |
| `src/input` | `key=value`, JSON, file, and standard-input parsing |
| `src/output` | Human output, JSON envelopes, and redaction |
| `packages/api-contract` | Environment-neutral contracts generated from OpenAPI |
| `packages/api-client` | Shared HTTP client used by the CLI |
| `skills/luna-devops` | The single paired Agent Skill |
| `scripts/cli` | Contract checks, packaging, artifact validation, and publishing |
| `openapi` | Platform contract snapshot protected by digest checks |

## Run from source

```bash
cd /path/to/luna-cli
pnpm install --frozen-lockfile

export LUNA_HOME="$(mktemp -d)"
pnpm exec tsx src/entry.ts version show
pnpm exec tsx src/entry.ts help catalog query=project limit=10 output=json interactive=false
```

Development, tests, and CI must use a temporary `LUNA_HOME` so they never read
or overwrite real credentials.

## Synchronize the platform contract

When the CLI is checked out under the platform repository:

```bash
pnpm --dir cli sync:openapi
LUNA_PLATFORM_ROOT=. pnpm --dir cli check:platform-coverage
```

For separate locations, point at the platform repository explicitly:

```bash
LUNA_PLATFORM_ROOT=/path/to/luna-devops pnpm check:platform-coverage
```

OpenAPI is the sole source of truth for ordinary JSON HTTP business commands.
SSE, WebSocket, and download flows use explicit protocol adapters. Browser
callbacks and webhook receivers must be classified by exact route instead of
being hidden by wildcard exclusions. `api request` is a diagnostic tool and
does not count as command coverage.

## Agents and the Skill

Agents always use machine output:

```bash
luna help catalog all=true limit=100 output=json interactive=false agent=true
luna help command path=registry.get-registries output=json interactive=false agent=true
```

Agents parse JSON from `stdout` and treat `stderr` as diagnostics. The Skill
describes intent routing, task order, and security boundaries; machine Help is
the source of truth for commands and parameters.

After changing commands, parameters, risk metadata, capability boundaries, or
the Skill, run:

```bash
pnpm check:skills
```

## Verification

Run these commands in the CLI repository:

```bash
pnpm check
pnpm check:release-scripts
pnpm check:contract
pnpm check:skills
LUNA_PLATFORM_ROOT=/path/to/luna-devops pnpm check:platform-coverage
```

See the complete
[`docs/cli-spec.md`](https://github.com/LiteyukiStudio/luna-cli/blob/main/docs/cli-spec.md)
for the technical specification.
