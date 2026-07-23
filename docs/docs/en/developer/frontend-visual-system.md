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

## Forms and action areas

- Settings forms should normally stay within `max-w-3xl` to `max-w-4xl`; short fields must not expand indefinitely with the content region.
- Related switches may use lightly inset option groups. Group longer forms by business meaning instead of placing every field at equal weight inside one large Card.
- Page-level submit and cancel actions use `FormActions`. Buttons keep their natural width and align right on desktop, becoming full width only on mobile.
- Tabs within the same settings page must place save actions consistently. The default position is the end of the current form; do not mix toolbar saves in some tabs with bottom saves in others.
- Long forms use a top divider before the action area. Dialogs continue to use `DialogFooter`; focused login and registration flows may retain full-width submit buttons.
- A button must not stretch across the form merely because its parent uses CSS Grid.

## Loading and empty states

Choose a shared skeleton that matches the page structure:

- `AppLoadingState` for session and public configuration bootstrap;
- `DataListSkeleton` for resource headers, rows, and pagination geometry;
- `OverviewSkeleton` for attention areas, metrics, and primary/secondary content;
- `SettingsSkeleton` for tabs, labels, and fields;
- `TemplateGridSkeleton` for marketplace tools and template grids;
- `ToolViewportSkeleton` for iframes, logs, topology, and terminal workspaces.

Once the application shell is available, replace only the content region. Do not place a single “Loading” line inside a large Card. `DataList` hides pagination when `total === 0`. First-time configuration states should offer a clear next step, while filtered empty results stay compact and provide a way to clear conditions.

## Actions and mobile workflows

- Keep one solid primary action per page or tab; use outline, ghost, or menus for peer actions.
- Dashboard and overview risks must use semantic `danger`, `warning`, `success`, or `info` tones with explicit text.
- When desktop filters exceed four fields, disclose them through a Sheet or popover on mobile while keeping search, filter entry, and refresh on the main page.
- Use `DataListColumn.mobile` to define retained columns for frequent mobile lists and move secondary metadata into the primary cell. Reserve horizontal scrolling for genuinely complex resource tables.
- Developer tools and other fixed or sticky controls must avoid dialogs, sheets, toasts, pagination, and sticky action columns.

## Technical terminology

- Use `Git Provider` for external source-control platforms and `OIDC Provider` for identity providers. Do not alternate between generic “provider”, “platform”, and product-specific labels within the same workflow.
- Use “permission scope” for token and protocol permissions, while “resource scope” remains the visibility boundary of a platform resource.
- Repository fields use “Owner” and “Repository” consistently. Preserve the original casing of protocol fields, API enums, commands, paths, and environment variables.
- OIDC claim names should keep their protocol wording, such as “Group Claim” and “Email Claim”, with contextual help where their meaning is not obvious.
- Keep capitalization and singular/plural forms consistent across tabs, tables, dialogs, and empty states.

## Verification

Changes to shared visual components, page templates, or theme tokens must check at least:

- light and dark modes;
- the default interaction theme and one non-default theme;
- widths of 1440px, 1024px, and 390px;
- empty states, error states, horizontal overflow, and long text.

A visual refactor must preserve existing query, authorization, validation, submission, and routing behavior.
