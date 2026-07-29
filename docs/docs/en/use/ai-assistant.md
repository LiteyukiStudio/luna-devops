# Use the embedded AI assistant

The Luna DevOps AI assistant explains build, release, Pod, Gateway, certificate, and notification problems and turns state spread across the console into a durable diagnostic timeline. It acts as the signed-in user, gains no extra permissions, and cannot bypass project roles, OAuth scopes, or Step-up MFA.

## Open the assistant

The assistant entry is rendered only when the deployment switch, site switch, Agent health, model configuration, cost limits, and current-user access all allow it. Unknown capability state, an unavailable Agent, or incomplete configuration keeps the entry hidden.

Desktop uses a draggable and resizable floating window. Mobile uses a full-screen view. Closing the window, refreshing the page, or losing the connection only removes the browser subscription; it does not cancel a durable run.

## Read the timeline

The timeline separates three content types:

- **Thinking** contains only a provider-approved reasoning summary or platform status explicitly labelled as execution progress. Hidden reasoning is never shown.
- **Message** contains user input and assistant answers.
- **Tool Call** records a platform operation. It is collapsed by default and reveals sanitized arguments, result, status, and duration.

Streaming Thinking uses a three-line viewport that follows the latest summary. Scrolling upward pauses following until the user returns to the latest position.

## Page actions

Some tool calls provide actions such as opening release events, viewing a Pod, or selecting the deployment tab. These actions use a versioned declarative UI Action registry and can target only registered Luna routes, tabs, filters, and dialogs. They cannot execute arbitrary scripts, DOM selectors, or external URLs.

Form actions may prefill allowed non-secret fields but never save automatically. Saving still uses the normal business API, authorization checks, and audit trail.

## Approval and MFA

A write or high-risk operation pauses the run and shows:

- the exact tool and operation;
- the target project space, application, environment, or resource;
- the complete bound arguments;
- expected impact and risk.

Approval is valid only for the current argument hash. Changed arguments require a new approval. When Step-up MFA is required, approval does not replace MFA; after verification, the run is queued again and resumes from its checkpoint.

## Conversations and recovery

Conversations live in the Agent-owned `ai` schema and are not stored in browser LocalStorage. Reopening a conversation first loads the complete Timeline and then subscribes from durable event cursors. Duplicate events are removed by event, Item, Part, and Tool Call ID.

Deleting a conversation physically cascades to its messages, runs, events, tool calls, approvals, and checkpoints. There is no soft-delete recovery window.

## Security boundary

- The browser connects only to Luna API, never directly to the model or `luna-agent`.
- The Agent can call only allowlisted OpenAPI operations.
- Luna API recalculates authorization for every tool call.
- Secrets, tokens, cookies, authorization headers, and kubeconfigs do not enter normal conversations, SSE, or model context.
- Tool results and model output pass through the shared redaction pipeline before reaching the browser.

