# Console visual system

Luna DevOps adds page-level visual primitives on top of shadcn/ui and Tailwind CSS. These primitives only define hierarchy, layout, and visual semantics. They must not own queries, authorization, or submission logic.

## Design tokens

Tailwind v4 tokens live in `web/src/styles/design-tokens.css`, while `brand-themes.css` continues to own the brand scales. Components consume semantic utilities instead of binding to concrete color values or repeating page dimensions.

| Responsibility | Tailwind utility / token | Scope |
| --- | --- | --- |
| Surfaces and text | `surface-*`, `foreground`, `muted-*` | Canvas, solid containers, and supporting content |
| Brand interaction | `primary-*` | Primary actions, links, focus, and selection |
| Status | `success-*`, `warning-*`, `danger-*`, `info-*` | Status feedback independent from brand choice |
| Separators | `separator-strong/subtle` | Section boundaries and repeated-item dividers |
| Radius | `rounded-control/container/feature` | Controls, regular containers, and feature containers |
| Spacing | `gap-inline/field/group/section`, `p-group/section` | Inline content, fields, related groups, and major sections |
| Page gutters | `px-page-inline`, `py-page-block` | Responsive mobile, medium, and desktop values owned by the token |
| Motion | `duration-fast/standard/slow`, `ease-standard/emphasized` | Control feedback, surface transitions, and emphasized movement |
| Elevation | `shadow-raised/overlay` | Interactive elevation and overlays |

Shared layout and semantic components own token consumption. Business pages compose `PageShell`, `PageChrome`, `Surface`, `Section`, `MetricGroup`, `DataList`, and `FormActions` instead of copying full component recipes through `@apply` or introducing page-specific token sets. shadcn primitives retain their own sizing so page-level tokens do not override accessibility or interaction contracts.

Use `duration-fast` for control feedback, `duration-standard` for card color, shadow, and subtle movement, and reserve `duration-slow` for larger or staged transitions. Regular color changes use `ease-standard`, while movement or scale emphasis uses `ease-emphasized`; business pages must not introduce arbitrary millisecond values. When reduced motion is enabled, global styles zero animation and transition durations, and transforms or scaling should be guarded with `motion-safe`.

## Page templates

Classify a new page before choosing its root layout:

| Page type | `PageShell` width | Recommended structure |
| --- | --- | --- |
| Resource list | `full` | `PageToolbar` + `DataList` |
| Dashboard or overview | `content` | attention area + `MetricGroup` + `Section` |
| Settings | `settings` | `ContentTabs` + `Section` / `Surface` |
| Logs, terminals, or topology | `tool` | tool bar + inset workspace |

`DataList` is already the list shell and should not be wrapped in another Card. Its shell uses `Card padding="none"` so the toolbar, header, body, and pagination own their internal spacing without duplicating the default `p-section` around the table. Keep one solid primary action per page by default. Search, filters, sorting, and refresh actions belong in `PageToolbar` or `ContentTabs.tools`.

## Surface hierarchy

Use semantic tokens instead of choosing ad hoc backgrounds for business containers:

- `surface-base`: page background.
- `surface-raised`: primary content and elevated surfaces.
- `surface-subtle`: weak grouping and hover states.
- `surface-inset`: code, logs, diagnostics, and filter workspaces.

Prefer `Surface` and `Section` for business sections. `Card` remains a base component for independent repeated items, but it is not the default wrapper for all content and should not create hierarchy-free Card nesting. Regular business containers, metric groups, notices, and resource display cards do not draw an outer border; solid surfaces and radius establish hierarchy. Row separators, split panes, form controls, and local structures that communicate ownership retain semantic boundaries.

Overlays use `shadow-overlay`. Primary surfaces that need gentle separation from the page use `shadow-raised`.

## Color responsibilities

Colors have three distinct roles:

1. Luna brand colors belong to the logo and fixed brand graphics.
2. Interaction theme colors belong to buttons, focus, selection, and tab indicators, and may follow site or user preferences.
3. Success, warning, danger, and information states use fixed semantic tokens and never change with the personal theme.

Use `StatusBadge`, `StatusValueBadge`, or `Notice` for status. Business pages should not compose state colors directly with `red-*`, `amber-*`, or `green-*` classes. Third-party brand icons, terminals, and centrally managed chart palettes are exceptions.

The console also supports Standard and Minimal interface styles. Standard blends a small amount of the brand border color into a neutral surface: light mode stays close to white, dark mode stays close to black, and progressively stronger blends distinguish navigation hover and active states. Minimal maps the large canvas, sidebar, and weak navigation surfaces completely back to neutral white/gray surfaces without changing primary actions, links, focus rings, tab indicators, or semantic status colors. The app root resolves the preference once and exposes it through `data-interface-style`; business pages must not read the account preference and introduce their own style branches.

Color themes support stable multi-color and single-color presets. Multi-color themes expose four semantic roles: `primary`, `theme-secondary`, `theme-supporting`, and `theme-highlight`. The layout layer maps the latter roles to selection surfaces and separators, while `workspace-background` remains a low-saturation solid surface. Business components consume only semantic tokens such as `workspace-background`, `theme-selection-surface`, and `separator-*`. Single-color themes resolve the final three roles back to the primary hue, so parallel business implementations are unnecessary. Theme choices use one circular palette language. Composite swatches use precise SVG paths: the primary color occupies the lower-right 50% semicircle behind a 45-degree dividing axis, while the other three colors divide the remaining half evenly. A primary-color base circle absorbs antialiasing seams, so `conic-gradient` must not be used for composite swatches. Single-color themes remain solid circles. Light, dark, and system modes only control appearance and live under personal account settings; they are not multiplied into separate color-theme variants.

## Spacing and density

- The authenticated content canvas uses spacious horizontal and compact vertical responsive padding: `px-8 py-4` on mobile, `px-12 py-6` on medium screens, and `px-16 py-8` on desktop. The topbar uses the same horizontal padding. `PageShell` owns only maximum width and section gaps; it must not add `mx-auto` or horizontal padding, so titles, dashboards, the marketplace, lists, and settings share one left baseline. Tool workspaces that need to fill available space should do so inside their viewport structure rather than overriding global page breathing room.
- On desktop, the page title belongs to the content workspace instead of a separate full-width topbar. It shares the global content-padding baseline with the body, tabs, and tools, using compact vertical spacing to read as one navigation group. Mobile retains a topbar containing the sidebar trigger and page title.
- Page headers consistently use `PageChrome`: the first row places the title on the left and page tools on the right and keeps a stable minimum height so switching to a tab without tools does not move the navigation vertically. Hierarchical pages may provide one back-navigation row below the title; it keeps link semantics and its destination is generated by the shared layout. An optional final row appears only when tabs are provided, and pages without tabs retain no empty navigation row. `ContentTabs` owns tab state and content switching and delegates its optional navigation and tools to `PageChrome`; on smaller screens, tools fall back into the document flow while back navigation remains below the title.
- `DataList` uses the same solid surface for its container and header instead of a full-width gray block. The table track is inset by `spacing-group` on both sides so content never touches the rounded container edge. The line below the header, separators between rows, sticky action-column boundaries, and skeleton rules all use the clearer `separator-strong`, keeping every table region at one visual weight; no rule is drawn above the header or below the final row. Row hover uses a rounded semantic surface and blends adjacent separators into it. Sticky action headers continue to inherit the same surface.
- The sidebar brand area is 72px tall; its 40px logo keeps 16px of space from both the top and left edges to maintain balanced horizontal and vertical insets.
- Content tabs align the neutral baseline and active primary indicator to the same bottom coordinate, with rounded line ends. Tab triggers do not draw a second bottom border, preventing a visible gap between the indicator and baseline.
- `SearchSelect` and `SearchMultiSelect` use `size="default" | "sm"` to control trigger, search field, option typography, vertical spacing, icons, and checkbox dimensions as one variant. Dense filter areas use `sm`; regular forms keep `default`. Business pages must not override those internal elements independently.
- The `DataList` Card shell has no border. Its table viewport uses the shared `TableFrame` with one native semantic border and one rounded clipping boundary to communicate local ownership. Do not simulate that border with nested backgrounds and padding because sticky headers can expose antialiasing seams at rounded corners. Inside the table, keep only the necessary separators between data rows; do not repeat rules around the toolbar, final row, or pagination.
- Place list search, filtering, sorting, and refresh controls in the `DataList` header toolbar and align them from the left. When the page title already identifies the list context, do not repeat labels such as “User list” inside the list shell. Use `DataList.title` only when multiple independent lists on the same page need explicit differentiation. Page-level primary actions such as create belong on the right side of the `PageChrome` title row. Do not add a separate query toolbar above the list. Use `DataList.toolbar` for filters, sorting, and refresh controls beyond the built-in search field.
- Primary forms, settings panels, and account panels use `p-6` by default. Catalog cards and compact metric cards may use `p-4` or `p-5`. `DataList`, logs, topology, terminals, and iframe shells use `p-0` and let their internal structure own spacing. Do not combine parent padding with compensating child margins in the same container.
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
- The authenticated console uses a layout-level `primary-subtle` background across every page header and full content canvas. Page components must not imitate full bleed with negative spacing or oversized decorative wrappers. Regular business `Surface` and `Card` containers use transparent borders and solid flat surfaces without persistent shadows. Shadows are reserved for dialogs, overlays, explicit raised surfaces, and interaction hover. Table rows, form controls, status feedback, and local ownership boundaries retain semantic borders where clarity requires them.
- The authenticated app root uses the low-saturation `workspace-background`, while the desktop sidebar remains transparent and directly inherits that global canvas without its own fill, right divider, or menu-group rules. Group labels and vertical spacing distinguish navigation categories. Navigation hover uses the light solid `sidebar-nav-hover`, while selection uses the deeper `sidebar-nav-active`; neither state uses a horizontal gradient whose trailing edge can disappear into the canvas, and selection must always carry more visual weight than hover. In Standard mode, `separator-*` may receive a restrained tint from the theme secondary role; Minimal mode keeps separators neutral. The mobile drawer remains an opaque themed overlay.
- Interface style resolves in this order: personal preference, platform default, then Standard. An empty account preference keeps following the platform; an explicit Standard or Minimal choice overrides it. New pages use the existing surface and theme tokens and must not add conditional classes specifically for Minimal mode.
- Sidebar group labels use a smaller supporting type size, regular weight, and lower contrast than actionable navigation items. Category labels such as Workbench, Resources, System Management, and Personal must not compete with clickable destinations.
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
- Organize dashboards in this reading order: risk summary and active metrics, recent operational flow with platform readiness, then frequent resource entry points. Normal zero-value metrics may be subdued, empty activity regions must not preserve large fixed-height blanks, and frequent resources use a responsive grid with an explicit “view all” path for longer collections.
- Use neutral solid surfaces for large dashboard regions. Reserve semantic color for exception icons, critical values, and necessary status badges instead of tinting notices, metric tiles, and labels simultaneously. `Notice` and `MetricItem` support `variant="neutral"` and `surface="neutral"` to retain semantic meaning with less visual noise.
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
