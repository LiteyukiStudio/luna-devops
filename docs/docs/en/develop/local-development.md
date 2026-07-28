# Local Development Guide

Use this setup when changing code, debugging APIs, or checking frontend interactions. If you only want to try the platform, follow the Docker Compose guide under Start instead.

## Recommended topology

For day-to-day work, split the processes like this:

- PostgreSQL, Redis, and worker run through `docker-compose-dev.yaml`.
- API runs on the host for Go debugging.
- Web runs on the host for Vite hot reload.

```bash
docker compose -f docker-compose-dev.yaml up -d --build
go run ./cmd/api
pnpm --dir web install
pnpm --dir web dev
```

## Backend entry points

- `cmd/api`: HTTP API, webhooks, OAuth callbacks, permission entry points, and task enqueueing.
- `cmd/worker`: async tasks such as builds, deployments, status sync, certificates, and cleanup.
- `internal/api`: HTTP handlers and response models.
- `internal/authz`: centralized authorization rules for platform roles, project roles, permission actions, and Access Token scopes.
- `internal/model`: GORM data models.
- `internal/provider`: adapters for Git, registries, Kubernetes, DNS, and other external platforms.
- `internal/worker`: async task runners.

Handlers receive input, enter authorization, and write responses. Put business rules in services, database access in repositories, and Git, Registry, or Kubernetes calls in providers. New authorization checks should reuse the actions and role matrix in `internal/authz` instead of repeating role checks across handlers.

## Frontend entry points

- `web/src/pages`: page modules.
- `web/src/components/ui`: shadcn/ui primitives.
- `web/src/components/common`: shared business components.
- `web/src/api`: API client and DTO types.
- `web/src/i18n`: Chinese and English copy.

Shared modules under `web/src` must use `@/` root imports. User-visible copy must go through i18n.

Production images embed the frontend build into the API. `index.html` uses revalidation, Vite `assets/` files use one-year immutable caching, and non-hashed public assets use short caching.

## Docs site

`docs/` is the Rspress documentation site. When a feature, flow, or user experience changes, update the user docs here as part of the change.
