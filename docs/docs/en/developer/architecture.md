# Local Development

Development mode is for changing code, debugging APIs, and validating UI interactions. If you only want to deploy the platform, start with the Docker Compose quick deploy page.

## Recommended topology

For daily development:

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
- `internal/model`: GORM data models.
- `internal/provider`: adapters for Git, registries, Kubernetes, DNS, and other external platforms.
- `internal/worker`: async task runners.

Handlers parse parameters and shape responses. Business logic belongs in services, data access in repositories, and external systems in providers.

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
