# Maintaining the Docs Site

The docs site uses Rspress 2 and lives in `docs/`.

## Commands

```bash
pnpm --dir docs install
pnpm --dir docs dev
pnpm --dir docs build
pnpm --dir docs preview
```

## Structure

```text
docs/
  rspress.config.ts
  theme/
  docs/
    public/
    zh/
    en/
```

`zh/` and `en/` use the same directory structure. Add, remove, or move pages in both languages together.

## Assets

- Logo: `docs/docs/public/luna-devops-logo.svg`
- Mascot: `docs/docs/public/brand/mascot-luna-catgirl-alpha.webp`

Assets come from the main frontend brand resources and are published as static docs assets.

## Writing style

- On user-facing pages, explain what to do now before explaining why the design works that way.
- Commands should be directly copyable.
- Start pages should help users deploy first; Use pages explain product capabilities; Develop pages cover code and contribution workflows.
- Dangerous operations must state their impact.
- Keep sentences short and avoid internal project-report language. A friendly tone is welcome, but accuracy still comes first.
