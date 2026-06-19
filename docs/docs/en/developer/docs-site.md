# Docs Site

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

`zh/` and `en/` should stay structurally aligned so bilingual maintenance remains simple.

## Assets

- Logo: `docs/docs/public/liteyuki-logo.svg`
- Mascot: `docs/docs/public/brand/mascot-liteyuki-catgirl-alpha.webp`

Assets come from the main frontend brand resources and are published as static docs assets.

## Writing style

- For users, explain what to do before explaining why it is designed that way.
- Commands should be directly copyable.
- Start pages should help users deploy first; Use pages explain product capabilities; Develop pages cover code and contribution workflows.
- Dangerous operations must state their impact.
- Friendly is welcome, but accuracy comes first.
