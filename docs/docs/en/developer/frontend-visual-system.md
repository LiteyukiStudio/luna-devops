# Console visual system

Luna DevOps adds page-level visual primitives on top of shadcn/ui and Tailwind CSS. These primitives only define hierarchy, layout, and visual semantics. They must not own queries, authorization, or submission logic.

## Page templates

Classify a new page before choosing its root layout:

| Page type | `PageShell` width | Recommended structure |
| --- | --- | --- |
| Resource list | `full` | `PageToolbar` + `DataList` |
| Dashboard or overview | `content` | attention area + `MetricGroup` + `Section` |
| Settings | `settings` | `ContentTabs` + `Section` / `Surface` |
| Logs, terminals, or topology | `tool` | tool bar + inset workspace |

`DataList` is already the list shell and should not be wrapped in another Card. Keep one solid primary action per page by default. Search, filters, sorting, and refresh actions belong in `PageToolbar` or `ContentTabs.tools`.

## Surface hierarchy

Use semantic tokens instead of choosing ad hoc backgrounds for business containers:

- `surface-base`: page background.
- `surface-raised`: primary content and elevated surfaces.
- `surface-subtle`: weak grouping and hover states.
- `surface-inset`: code, logs, diagnostics, and filter workspaces.

Prefer `Surface` and `Section` for business sections. `Card` remains a base component for independent repeated items, but it is not the default wrapper for all content and should not create hierarchy-free Card nesting.

Overlays use `shadow-overlay`. Primary surfaces that need gentle separation from the page use `shadow-raised`.

## Color responsibilities

Colors have three distinct roles:

1. Luna brand colors belong to the logo and fixed brand graphics.
2. Interaction theme colors belong to buttons, focus, selection, and tab indicators, and may follow site or user preferences.
3. Success, warning, danger, and information states use fixed semantic tokens and never change with the personal theme.

Use `StatusBadge`, `StatusValueBadge`, or `Notice` for status. Business pages should not compose state colors directly with `red-*`, `amber-*`, or `green-*` classes. Third-party brand icons, terminals, and centrally managed chart palettes are exceptions.

## Spacing and density

- Use `gap-6` between major page sections.
- Use `gap-4` between related sections.
- Use `gap-3` for fields and tool groups.
- Use `gap-2` for inline buttons, badges, and icon-label pairs.
- Use `gap-1` for compact metadata.

Prefer standard Tailwind spacing and width tokens. Do not introduce arbitrary pixel values for local visual adjustments.

## Verification

Changes to shared visual components, page templates, or theme tokens must check at least:

- light and dark modes;
- the default interaction theme and one non-default theme;
- widths of 1440px, 1024px, and 390px;
- empty states, error states, horizontal overflow, and long text.

A visual refactor must preserve existing query, authorization, validation, submission, and routing behavior.
