# AI Assistant

The AI assistant uses the current page context and your platform permissions to query resources, explain status, and help with routine operations.

## Get started

A platform administrator first configures these values under **Global Settings → AI Assistant**:

- Provider URL
- API key
- Default model
- Optional fallback model
- Users allowed to access the assistant

After the connection test passes, the assistant entry appears in the lower-right corner of the console.

## Conversations

In the assistant you can:

- Describe what you want to inspect or change
- Create, switch, rename, and batch-delete conversations
- Switch conversations while another response is running
- Stop a response at any time

An empty new conversation is not saved repeatedly. Conversations are named automatically until you rename one manually; a manually chosen title is never overwritten.

## Queries and operations

The assistant has the same platform permissions as you. It cannot read or change a project space or resource that your account cannot access.

A request may require several resource queries. The assistant continues from each tool result and explains or corrects failed queries instead of treating an unfinished step as the final response.

Read-only queries run directly. High-risk operations such as deletion, release, or permission changes show a confirmation card first. You can:

- Approve this operation
- Reject it
- Allow subsequent operations of the same kind in the current conversation

Approval never expands your platform permissions. The backend still performs the final authorization check.

## Suggestions and navigation

After a response, the assistant shows a small set of next steps only when the model produces valid suggestions that are directly relevant to the current task. A suggestion may:

- Continue the conversation
- Open a related platform page
- Run one clearly described follow-up operation

For reading tasks, the assistant may include internal links. When it needs a decision, it prefers selectable options instead of moving you away from the current page.
If the model does not return reliable contextual suggestions, the platform omits the suggestion area instead of filling it with fixed generic choices.

## Interaction cards

The assistant can organize resource candidates, comparisons, diagnostics, and configuration fields into cards. Continue or action buttons stay disabled until required fields are valid.

When a form continues the conversation, only the non-sensitive fields declared by the card are included. Passwords, tokens, and secrets are never added to the message history. Any resulting operation still goes through platform authorization and required confirmation.

## Page context and privacy

The assistant receives structured context such as the current route, page type, and selected resource IDs. Passwords, tokens, and secrets shown or entered on a page are not included in that context.

Tool details are collapsed by default. Expand them to inspect redacted parameters, execution status, and results.

## Troubleshooting

### The assistant is missing

Ask a platform administrator to verify that the assistant is enabled, the Provider connection test passes, and your account is included in the allowed audience.

### The model is unavailable

Check Provider quota, model name, and API key. If a fallback model is configured, switch to it and retry.

### A tool failed

Expand the tool details and keep the request ID. Common causes include a resource changing, missing permission, an unavailable dependency, or an operation that needs confirmation again.
