# Read This Before Contributing

## Before changing code

- Read existing code and docs before editing.
- Do not commit, push, or switch branches unless explicitly requested.
- Move one small goal through one traceable cycle.
- Update the `docs/` documentation site when adding features or changing flows.
- Update `TODO.md` when the plan, acceptance criteria, or status changes.

## Frontend

- Use `pnpm`.
- Prefer shadcn/ui for primitives.
- Use React Hook Form + Zod for forms.
- User-visible text must go through i18n.
- Lists should reuse the shared list component.
- Status must use semantic badges.

## Backend

- Use PostgreSQL, not SQLite.
- API startup runs embedded `migrations/*.up.sql`; legacy databases without `schema_migrations` are adopted at 008 before later migrations run.
- The running API serves the bundled OpenAPI document at `/openapi.yaml` and Swagger UI at `/swagger`.
- Do not store secrets or tokens as plaintext in business tables.
- External platform capabilities are adapted through backend providers, services, and APIs. The frontend must not orchestrate third-party APIs.
- Long-running work goes to workers, not synchronous HTTP requests.

## How to verify a change

Run only the directly relevant checks for a small change. When work crosses business domains or touches authentication, authorization, secrets, migrations, or deployment runtime, run the complete verification set and, whenever possible, walk through the real interaction in a browser.

Backend development and release checks require exactly Go `1.26.5`; the version is pinned in `.go-version`, `go.mod`, and the Dockerfile builder image. Run the release-candidate gate from a clean Git worktree:

```bash
./scripts/release-check.sh
```

The release-quality gate verifies the Go version and `gofmt`, then runs all Go tests, `go vet`, race tests for critical packages, frontend tests/lint/build, the documentation build, high-severity pnpm dependency audits, `govulncheck`, and Helm lint/render. Before running it, set `AUTH_TEST_DATABASE_URL` to a PostgreSQL test database where temporary schemas can be created; CI starts PostgreSQL automatically. The script refuses to continue without that variable so migration and concurrent-authentication integration tests cannot be skipped silently. The regular Go and race suites run without that database variable, while the authentication and migration integration suites run once with `-count=1`. Any failure blocks the release. The script also refuses a worktree with modified or untracked files so the verified source matches the release candidate.

## Documentation experience

Documentation should save users from unnecessary work. Start by answering:

- What is the user trying to finish now?
- What is the shortest path?
- What should success look like?
- Where should they look first when it fails?

Architecture and internal boundaries belong in developer docs. They should not block a user from getting started.
