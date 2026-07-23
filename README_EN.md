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
  <a href="https://github.com/LiteyukiStudio/devops">GitHub</a>
  ·
  <a href="docs/docs/en/guide/deployment/kubernetes-helm.md">Helm</a>
  ·
  <a href="docs/docs/en/guide/deployment/docker-compose.md">Docker Compose</a>
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
| Frontend | Vite, React, TypeScript, Tailwind CSS, shadcn/ui, TanStack Query |
| Forms and UX | React Hook Form, Zod, i18next, react-i18next, Sonner |
| Delivery | Docker Compose, Helm, Kubernetes Job, BuildKit, Gateway API |
| Tooling | pnpm, uv, golang-migrate, OpenAPI |

## Quick Start

Start local dependencies:

```bash
docker compose -f docker-compose-dev.yaml up -d
```

Create local configuration:

```bash
cp .env.example .env
```

Run the backend:

```bash
go run ./cmd/api
go run ./cmd/worker
```

Run the frontend:

```bash
pnpm --dir web install
pnpm --dir web dev
```

The Vite dev server proxies `/api/v1` to `http://localhost:8080`.

## Deployment

Luna DevOps can run from containers, Helm, or a local binary workflow. Containerized deployment is recommended for real environments.

| Method | Best for | Entry point |
| --- | --- | --- |
| Kubernetes / Helm | Production-like Kubernetes or K3s clusters | [`charts/luna-devops`](charts/luna-devops) |
| Docker Compose | Single-node trial, small labs, release verification | [`docker-compose.yaml`](docker-compose.yaml) |
| Binary | Local debugging and source-level development | [`cmd/api`](cmd/api), [`cmd/worker`](cmd/worker) |

Start the published container images with Docker Compose:

```bash
cp .env.example .env
# Fill SECRET_ENCRYPTION_KEY, BOOTSTRAP_TOKEN, and REDIS_PASSWORD before first startup.
docker compose up -d
```

Build the complete stack from the current source tree:

```bash
docker compose -f docker-compose-build.yaml up -d --build
```

Install with Helm:

```bash
helm install luna-devops ./charts/luna-devops \
  --namespace luna-devops \
  --create-namespace
```

More deployment guides:

- [Kubernetes / Helm](docs/docs/en/guide/deployment/kubernetes-helm.md)
- [Docker Compose](docs/docs/en/guide/deployment/docker-compose.md)
- [Binary deployment](docs/docs/en/guide/deployment/binary.md)
- [Configuration reference](docs/docs/en/guide/configuration-reference.md)

## Configuration Notes

- `APP_ENV=development` enables local development conveniences.
- `APP_ENV=production` disables development defaults and requires administrator bootstrap.
- `SECRET_ENCRYPTION_KEY` must be stable in production. It protects stored tokens, registry credentials, OAuth secrets, and other sensitive values.
- `TRUSTED_PROXY_CIDRS` should include trusted reverse proxies or CDN egress ranges when Luna DevOps is behind a proxy.
- Worker build networking is configurable. Use restricted egress plus explicit allowlists when builds need to access private registries or mirrors.

For the full list of API and Worker options, use the [configuration reference](docs/docs/en/guide/configuration-reference.md).

## Repository Layout

```text
cmd/api                 API service entry point
cmd/worker              Async Worker entry point
internal/               Backend domains, providers, services, and models
migrations/             PostgreSQL migrations
openapi/                OpenAPI definitions
web/                    Vite + React console
web/public/             Public assets, logo, mascot, and favicon
docs/                   Rspress documentation site
notes/                  Product notes, engineering notes, and SOPs
charts/luna-devops      Helm chart
```

## Development

Common checks:

```bash
go test ./...
pnpm --dir web lint
pnpm --dir web build
```

Project conventions:

- Use `pnpm` for frontend dependencies.
- Use `uv` for Python tooling.
- Keep backend handlers thin; put business logic in services and external integrations in providers.
- Keep user-facing frontend text in i18n files.
- Update the documentation site when a feature or behavior changes.

## Assets

- Logo / favicon: [`web/public/luna-devops-logo.svg`](web/public/luna-devops-logo.svg)
- Mascot: [`web/public/brand/mascot-luna-devops.png`](web/public/brand/mascot-luna-devops.png)

## Documentation

- Public documentation: [luna-devops.liteyuki.org](https://luna-devops.liteyuki.org/en/)
- Product notes: [`notes/01-产品与一体化方案.md`](notes/01-产品与一体化方案.md)
- Code health SOP: [`notes/07-代码健康检查SOP.md`](notes/07-代码健康检查SOP.md)
- Development plan: [`TODO.md`](TODO.md)
- Agent and contribution rules: [`AGENTS.md`](AGENTS.md)
