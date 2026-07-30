# Use the embedded AI assistant

The Luna DevOps AI assistant explains build, release, Pod, Gateway, certificate, and notification problems and turns state spread across the console into a durable diagnostic timeline. It acts as the signed-in user, gains no extra permissions, and cannot bypass project roles, OAuth scopes, or Step-up MFA.

## Open the assistant

The assistant entry is rendered only when the deployment switch, site switch, Agent health, model configuration, cost limits, and current-user access all allow it. Unknown capability state, an unavailable Agent, or incomplete configuration keeps the entry hidden.

A platform administrator configures only three model values under Global Settings →
AI Assistant: an OpenAI-compatible HTTPS API URL, API key, and model name. The
platform selects the managed Provider automatically; there is no Provider type,
fallback model, pricing JSON, or Agent mode to configure. The API key is encrypted
in the platform secret store and never displayed when editing. Embedded credentials,
private networks, and reserved addresses remain blocked, while a valid public endpoint
does not need to be duplicated in the global domain allowlist.

Advanced runtime settings stay collapsed by default. Platform administrators may adjust
the model request timeout, complete Run timeout, and concurrent Runs per Agent instance
when needed. The authenticated internal configuration channel applies updates within
about 30 seconds without restarting containers. Polling, leases, and refresh cadence are
internal consistency details with safe built-in defaults rather than deployment variables.

Desktop uses a draggable and resizable floating window. Mobile uses a full-screen view. The launcher is draggable, and the browser remembers the window size, window position, and launcher position across reloads while keeping them inside the visible viewport. Closing the window, refreshing the page, or losing the connection only removes the browser subscription; it does not cancel a durable run.

The list button at the far left of the top bar opens the conversation list. The right side keeps only New Conversation and Close, clearly separating navigation from current-conversation actions. Closing restores the draggable launcher, so there is no separate minimized state with the same outcome. The New Conversation action is also available in the conversation list. Each conversation keeps its own draft, streaming timeline, run state, and SSE subscription. You can switch to another conversation and send a message while the first one is still generating. The list marks every generating conversation, not only the selected one.

The conversation list has an explicit bulk-selection mode. You can select individual conversations, select all visible conversations, and confirm one bulk deletion. Leaving selection mode clears its temporary state, so checkboxes do not permanently clutter normal browsing. Ownership is still verified for every deleted conversation.

Enter sends from the composer, while Shift+Enter inserts a newline. When a Chinese, Japanese, or other IME is still composing a candidate, Enter confirms that candidate without sending the message. Press Enter again after composition ends to send.

## Read the timeline

The timeline separates three content types:

- **Thinking** contains only a provider-approved reasoning summary or platform status explicitly labelled as execution progress. Hidden reasoning is never shown.
- **Message** contains user input and assistant answers.
- **Tool Call** records a platform operation. Its compact collapsed row shows only the tool name and a status badge on the right; expanding it reveals sanitized arguments, result, status, and duration. Internal maintenance tools such as automatic conversation naming are omitted from the user-facing timeline.

The timeline first groups blocks into user turns by `turnId`. Each turn shows its user message exactly once and then one assistant reply container. Inside that container, Thinking, intermediate Message, Tool Call, and final Message blocks retain their durable `timelineIndex` order. Text emitted before and after tool calls therefore remains part of one reply instead of becoming separate assistant bubbles, and live streams use the same topology as restored history.

User and assistant bubbles align to the right and left respectively and are capped at 78% of the message area, preserving a recognizable lane for the other participant. Short messages shrink to their content, while tables, code, and tool details still contain their own overflow inside the assistant bubble.

A Tool Call shows a spinning loader only while it is genuinely `running`. Failed, succeeded, canceled, skipped, approval-waiting, and MFA-waiting states use distinct static semantic icons so a terminal operation is never mistaken for ongoing work.

Model responses arrive incrementally over recoverable SSE instead of appearing only after the complete answer is ready. A three-dot wave is shown before the first visible delta; once output begins, the Message grows naturally with each delta. Streaming Thinking uses a smaller, compact three-line viewport that follows the latest summary and collapses when complete. Scrolling upward pauses following until the user returns to the latest position.

Assistant answers use compact message bubbles and render headings, paragraphs, lists, quotes, links, code blocks, and tables with GitHub Flavored Markdown. When an answer mentions a registered page or an existing resource, the Agent follows its built-in navigation Skill and emits `[label](/internal/path)`. The browser renders registered internal destinations in the primary color and uses React Router to navigate without reloading. Unknown internal paths, protocol-relative URLs, and script schemes are not clickable; external HTTP(S) links still open in a new tab.

The window and timeline scroll vertically only; tables and code blocks own their horizontal scrolling without widening the window or moving the whole conversation sideways. Raw HTML and external images returned by the model are not executed or loaded.

## Page actions

Inline internal Markdown links directly reference the page or resource being discussed; structured `create_options` provides multiple optional next steps. Both use the same registered route boundary. A link only navigates and never means access was granted or a business operation ran.

Some tool calls provide actions such as opening release events, viewing a Pod, or selecting the deployment tab. These actions use a versioned declarative UI Action registry and can target only registered Luna routes, tabs, filters, and dialogs. They cannot execute arbitrary scripts, DOM selectors, or external URLs.

Form actions may prefill allowed non-secret fields but never save automatically. Saving still uses the normal business API, authorization checks, and audit trail.

## Approval and MFA

Reads and low-risk writes are re-authorized by Luna API as the signed-in user and run immediately only when that user is allowed. Sensitive, destructive, or explicitly approval-required high-risk operations pause the run and show:

- the exact tool and operation;
- the target project space, application, environment, or resource;
- the complete bound arguments;
- expected impact and risk.

The user can Approve, Reject, or Approve all. Approval is valid only for the current argument hash and version. Changed arguments require a new approval. Approve all covers only calls already displayed and still pending in the current Run; it never covers future calls, another conversation, or changed arguments. When Step-up MFA is required, approval does not replace MFA; after verification, the run is queued again and resumes from its checkpoint.

The assistant can also present structured next-step options. An option may send a new
message, open a registered platform route, or request a controlled tool operation. A
tool request still passes through policy, argument validation, approval, and MFA; the
option click never executes a dangerous action directly.

`create_options` is a UI-only tool and does not read or mutate business data. Every normally completed response includes one user-intent prediction: the Agent derives 2-5 next-step options from the current question, the six most recent turns, page context, and trusted tool results, validates and durably stores them in the Timeline, and sends them to the browser over SSE. The UI renders them as always-visible Suggested next steps buttons instead of hiding them inside a collapsed generic Tool Call. If the Provider omits the tool, the Agent performs one required structured prediction pass; only a still-incompatible Provider falls back to a safe route-aware set so a completed Turn remains actionable. Each option has a stable ID, display label, optional description and tone, an independent repeat policy, plus exactly one action:

- **Send message** may use different button and submitted-message text and creates a normal new Turn.
- **Navigate** accepts only a registered route name, parameters, and query values. React Router navigates without a page reload, and arbitrary URLs are rejected.
- **Request operation** carries an operation ID, candidate arguments, and a user-readable request, but selecting it only creates explicit user intent; the browser never calls the business API directly.

Options are independent rather than mutually exclusive. Internal navigation is repeatable by default, so the user may navigate and then still choose a message or another suggestion. Sending a message and requesting an operation create new work; after success, only that option is disabled to prevent duplicate intent or a repeated non-idempotent request. Sibling options remain available, and a failed attempt does not consume the option.

The Agent also exposes a separate `navigate_to_route` UI tool. The model may use it only when the user explicitly asks to open, go to, or switch to a known page, or when an immediate route change is necessary and unambiguous. The Agent stores only the registered route name, parameters, and query. When the browser receives the completed Tool Call through live SSE, it validates the registry and performs one React Router navigation. Historical Timeline hydration, replayed events, and arbitrary URLs cannot trigger automatic navigation; uncertain next steps remain clickable options or Markdown links.

Authorization is layered. Reading options and creating the next Turn use the current browser session, CSRF protection, and conversation ownership. A real tool operation re-enters the Agent Tool Catalog, and Luna API recalculates the current user, project, OAuth scope, resource ownership, and latest authorization state. The Agent has no separate service-account authority: a registered action can succeed only when the user can perform it. Only high-risk operations require argument-hash-bound approval and, when configured, Step-up MFA. A model-generated, visible, or selected button never grants permission by itself.

## Conversations and recovery

Conversations live in the Agent-owned `ai` schema and are not stored in browser LocalStorage. Reopening a conversation first loads the complete Timeline and then subscribes from durable event cursors. Duplicate events are removed by event, Item, Part, and Tool Call ID.

Each visible text delta updates its durable Item and is recorded as a Run Event with stable `turnIndex` / `timelineIndex` ordering before Agent SSE and Luna API forward it to the browser. The client never uses the per-run event sequence to order messages across turns, so a new streamed answer cannot jump ahead of older messages and then move again when the final Timeline arrives. After a refresh, network interruption, or route change, the client resumes with the `Last-Event-ID` / `after` cursor. A terminal event closes only that conversation's subscription.

When the model service rejects authentication, runs out of quota, rate limits, times out, or becomes unavailable, the Agent persists only a stable error code instead of forwarding the upstream exception. The timeline renders a localized failure notice. Manual cancellation also leaves an explicit terminal notice so a run never appears to end silently.

The Agent registers platform tools with the model only when both `LUNA_API_BASE_URL` and `AI_INTERNAL_SECRET` are valid. This prevents the model from selecting a tool that the current Agent instance cannot execute. During local integration, Luna API and Agent must use the same `AI_INTERNAL_SECRET`; purpose-specific callback, signing, and encryption keys are derived automatically.

The tool set is also filtered by page scope. Platform-level pages expose platform capabilities such as `getDashboard`, `listProjects`, and `createProject`. Project events, builds, releases, gateways, and runtime tools are registered only when the structured page context contains a valid `projectId`; the model cannot expand that scope by supplying a different project ID.

For every new Turn, the Web client sends a versioned page-context envelope containing the registered route name and template, page kind, project and application IDs, active tab, allowlisted view state, selected resource IDs, available tabs, related internal routes, locale, time zone, and client timestamp. Tokens, callback targets, and other non-allowlisted query parameters are excluded. Luna API overwrites the locale and adds the server request time and whether the project boundary was authorized for the current session. This metadata helps the model understand the current workspace but never replaces authorization on each tool call.

The Agent also supplies the six most recent selected Runs as real `user` and `assistant` messages instead of flattening all history into one text blob. Each historical message has a length limit, while the current request, page context, and current tool results keep priority. Historical and page content remain untrusted data and cannot override system safety rules.

The stop button cancels only the active Run in the selected conversation. The Agent persists the `canceled` terminal state and immediately aborts a model stream owned by the current instance; another instance observes the canceled lease on its heartbeat and stops. Partial visible output remains in the Timeline instead of rolling back or reordering the conversation.

Creating a conversation reuses an unsent empty conversation in the current project scope, so repeated clicks do not accumulate empty records. Every model request includes the current title, title source, and turn index. A first turn with the default title must call the built-in `rename_conversation` tool. When an assistant-generated title substantially diverges from the new main topic, the model may call the tool again.

Renaming a conversation in the list durably changes its title source to `user`, which the list indicates with a lock icon. The Agent then removes the rename tool from that model request, and the database update condition independently rejects any attempted Agent rename. This protection does not depend on prompt compliance, so a manually chosen title is never overwritten.

Deleting a conversation physically cascades to its messages, runs, events, tool calls, approvals, and checkpoints. There is no soft-delete recovery window.

## Security boundary

- The browser connects only to Luna API, never directly to the model or `luna-agent`.
- The Agent can call only allowlisted OpenAPI operations.
- Luna API recalculates authorization for every tool call.
- Secrets, tokens, cookies, authorization headers, and kubeconfigs do not enter normal conversations, SSE, or model context.
- Tool results and model output pass through the shared redaction pipeline before reaching the browser.
