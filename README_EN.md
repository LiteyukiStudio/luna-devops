<p align="center">
  <img src="web/public/luna-devops-logo.svg" width="132" alt="Luna DevOps logo" />
</p>

<h1 align="center">Luna DevOps</h1>

<p align="center">
  A lightweight application delivery platform for small teams, businesses, and independent developers.
</p>

<p align="center">
  <a href="README.md">简体中文</a> · <strong>English</strong>
</p>

<p align="center">
  <img src="web/public/images/luna-devops-banner-v4.png" alt="Luna DevOps automated delivery pipeline banner" />
</p>

<p align="center">
  <a href="https://luna-devops.liteyuki.org/en/">Documentation</a>
  ·
  <a href="https://github.com/LiteyukiStudio/luna-devops">GitHub</a>
  ·
  <a href="docs/docs/en/start/install/kubernetes.md">Helm</a>
  ·
  <a href="docs/docs/en/start/install/docker-compose.md">Docker Compose</a>
</p>

## What Is Luna DevOps?

Luna DevOps connects source repositories, image registries, BuildKit, Kubernetes, gateway routes, certificates, releases, and billing into one delivery workflow.

The goal is simple: keep the product team focused on code, while the platform handles the repeatable steps required to build and expose a service.

```text
Repository
  -> Build image
  -> Push registry artifact
  -> Deploy to Kubernetes / K3s
  -> Create gateway route
  -> Track status, logs, release history, and usage
```

## Features

| Area | Included |
| --- | --- |
| Workspaces | Project spaces, applications, members, roles, and audit-oriented operations |
| Repositories | GitHub and Gitea account integration, repository binding, and webhook entry points |
| Builds | Worker-managed Kubernetes Jobs, rootless BuildKit, image tags, logs, and build records |
| Registries | Harbor, Gitea Registry, DockerHub, and generic OCI registry connections |
| Deployments | Kubernetes / K3s workloads, release records, status sync, and rollback-oriented history |
| Gateway | Gateway API / HTTPRoute, domains, access entries, and certificate automation |
| Operations | Events, notifications, application marketplace, billing, and platform settings |
| User experience | React console, i18n, light / dark / system theme, and embedded production frontend |

## Tech Stack

| Layer | Stack |
| --- | --- |
| Backend | Go, Gin, GORM, PostgreSQL, Redis, Asynq, client-go |
| AI Agent | Node.js 24, TypeScript, Fastify, LangGraph.js, PostgreSQL checkpoints |
| Frontend | Vite, React, TypeScript, Tailwind CSS, shadcn/ui, TanStack Query |
| Forms and UX | React Hook Form, Zod, i18next, react-i18next, Sonner |
| Delivery | Docker Compose, Helm, Kubernetes Job, BuildKit, Gateway API |
| CLI | TypeScript, Commander, Zod, i18next, npm / pnpm, Bun |
| Tooling | pnpm, uv, golang-migrate, OpenAPI |

## Quick Start

Start local dependencies:

```bash
docker compose -f docker-compose-dev-db.yaml up -d
```

Create local configuration:

```bash
cp .env.example .env
```

The development Compose files do not manage Luna DevOps processes. Run the four components in separate terminals:

```bash
# Terminal 1: API
go run ./cmd/api

# Terminal 2: Worker
go run ./cmd/worker

# Terminal 3: Agent
pnpm --dir luna-agent install
pnpm --dir luna-agent dev

# Terminal 4: Web
pnpm --dir web install
pnpm --dir web dev
```

The Vite dev server proxies `/api/v1` to `http://localhost:8080`.
See [`luna-agent/.env.example`](luna-agent/.env.example) for Agent integration settings. Also set `AI_ASSISTANT_AVAILABLE=true` and `AI_AGENT_BASE_URL=http://localhost:8091` in the root `.env`; `AI_INTERNAL_SECRET` must match in both files.

API, Worker, helper commands, and Agent default to `LOG_FORMAT=auto`: interactive terminals get readable console logs, while redirected output and containers get ANSI-free JSON. Use `LOG_LEVEL` to change verbosity or `LOG_COLOR=never` / `NO_COLOR` to disable color. OTel always receives structured records independently of terminal rendering.

## Luna CLI

Luna CLI manages Luna DevOps from a terminal with human-readable output and
JSON output for automation:

```bash
npm install --global @liteyuki/luna-cli
luna login
luna project get-projects
```

- [CLI documentation](https://luna-devops.liteyuki.org/en/use/cli/installation)
- [CLI GitHub repository](https://github.com/LiteyukiStudio/luna-cli)
- [Paired Agent Skill](https://github.com/LiteyukiStudio/luna-cli/tree/main/skills/luna-devops)

## Deployment

Luna DevOps can run from containers, Helm, or a local binary workflow. Containerized deployment is recommended for real environments.

| Method | Best for | Entry point |
| --- | --- | --- |
| Kubernetes / Helm | Production-like Kubernetes or K3s clusters | [`charts/luna-devops`](charts/luna-devops) |
| Docker Compose | Single-node trial, small labs, release verification | [`docker-compose.yaml`](docker-compose.yaml) |
| Binary | Local debugging and source-level development | [`cmd/api`](cmd/api), [`cmd/worker`](cmd/worker) |

The DockerHub images are published as `liteyukistudio/luna-devops`, `liteyukistudio/luna-worker`, and `liteyukistudio/luna-agent`.

Start the published container images with Docker Compose:

```bash
cp .env.example .env
# Fill SECRET_ENCRYPTION_KEY, BOOTSTRAP_TOKEN, and REDIS_PASSWORD before first startup.
docker compose up -d
```

The AI assistant is disabled by default. After configuring the Agent trust material and durable encryption key in `.env`, enable its explicit profile:

```bash
AI_ASSISTANT_AVAILABLE=true docker compose --profile ai up -d
```

Install with Helm:

```bash
helm install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace
```

More deployment guides:

- [Kubernetes / Helm](docs/docs/en/start/install/kubernetes.md)
- [Docker Compose](docs/docs/en/start/install/docker-compose.md)
- [Binary deployment](docs/docs/en/start/install/binary.md)
- [Configuration](docs/docs/en/start/configuration.md)

## Configuration Notes

- `APP_ENV=development` enables local development conveniences.
- `APP_ENV=production` disables development defaults and requires administrator bootstrap.
- `SECRET_ENCRYPTION_KEY` must be stable in production. It protects stored tokens, registry credentials, OAuth secrets, and other sensitive values.
- `TRUSTED_PROXY_CIDRS` should include trusted reverse proxies or CDN egress ranges when Luna DevOps is behind a proxy.
- Worker build networking is configurable. Use restricted egress plus explicit allowlists when builds need to access private registries or mirrors.

For the full list of API and Worker options, see [Configuration](docs/docs/en/start/configuration.md).

## Repository Layout

```text
cmd/api                 API service entry point
cmd/worker              Async Worker entry point
luna-agent/             Independent AI Agent, orchestration graph, tool catalog, and durable runtime
internal/               Backend domains, providers, services, and models
migrations/             PostgreSQL migrations
openapi/                OpenAPI definitions
web/                    Vite + React console
web/public/             Public assets, logo, mascot, and favicon
docs/                   Rspress documentation site
docs-internal/                  Internal engineering docs (standards, proposals, records)
charts/luna-devops      Helm chart
```

The optional local `/cli/` directory is ignored by Git and exists only for a
standalone CLI checkout during integration development.

## Development

Common checks:

```bash
go test ./...
pnpm --dir web lint
pnpm --dir web build
```

Project conventions:

- Use `pnpm` for frontend dependencies.
- `web/`, `docs/`, `tests/`, and `luna-agent/` each own their manifests and lockfiles. There is no cross-directory root pnpm workspace; any pnpm project settings stay inside the owning directory.
- Use `uv` for Python tooling.
- Keep backend handlers thin; put business logic in services and external integrations in providers.
- Keep user-facing frontend text in i18n files.
- Update the documentation site when a feature or behavior changes.

## Assets

- Logo / favicon: [`web/public/luna-devops-logo.svg`](web/public/luna-devops-logo.svg)
- Mascot: [`web/public/brand/mascot-luna-devops.png`](web/public/brand/mascot-luna-devops.png)

## Documentation

- Public documentation: [luna-devops.liteyuki.org](https://luna-devops.liteyuki.org/en/)
- Product notes: [`docs-internal/产品概要.md`](docs-internal/产品概要.md)
- Internal engineering docs index: [`docs-internal/README.md`](docs-internal/README.md)
- Code health SOP: [`docs-internal/代码检查流程.md`](docs-internal/代码检查流程.md)
- Development plan: [`TODO.md`](TODO.md)
- AI agent rules: [`AGENTS.md`](AGENTS.md)
- Contributing guide: [`CONTRIBUTING.md`](CONTRIBUTING.md)

## License

Luna DevOps is open source under the [MIT License](LICENSE). You may use, copy, modify, merge, publish, and distribute the project as long as copies or substantial portions retain the original copyright and license notice.

The project is provided “as is,” without express or implied warranty. Third-party dependencies, external services, and third-party brand assets remain subject to their own terms. The MIT License does not grant trademark rights to the Luna DevOps or Liteyuki Studio names and logos. See the [license guide](docs/docs/en/reference/license.md) for details.
