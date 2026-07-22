# First Time in the Console

After the platform starts, complete the few settings you actually need. There is no need to connect every external system at once. If you can sign in and create a project space, you are ready to prepare a runtime.

## Sign in or bootstrap

The complete Compose stack starts the API in production mode and does not create a fixed development administrator. On the first visit, initialize an administrator with the deployment's `BOOTSTRAP_TOKEN`. Development account hints appear only when a local environment explicitly sets `APP_ENV=development`.

Local-account sign-in and first-administrator bootstrap both create a server-side session that lasts at most 24 hours. "Keep me signed in" is off by default, so the session cookie has no persistent lifetime and disappears when the browser closes. Enabling it on a trusted device adds a per-user HttpOnly remember cookie with an absolute 30-day lifetime. After the session expires, choosing that recent account rotates the token inside the same token family and creates a new session, but rotation never extends the family's original 30-day deadline or treats remember recovery as a new password/OIDC primary authentication. Each family keeps only its latest session. Reuse of an old token is treated as replay and revokes the entire family's remember tokens, sessions, and Step-up assertions; signing out from a remembered session revokes that family as well. The browser keeps display metadata for at most three recent accounts, but never stores passwords, tokens, or session cookies. Disabling the account, changing its password, or changing its role revokes the account's related authentication state.

Every person has one platform user record. A password and an OIDC identity are simply different sign-in methods that can be attached to that account. The first OIDC sign-in no longer creates a separate kind of “OIDC user.” The same account may bind external identities and, when policy allows it, add a local password. Before the final OIDC identity can be removed, the account must already have another usable sign-in method.

Platform administrators control email registration and OIDC registration independently under Site Settings → Registration & Email. Email registration is off by default and OIDC registration is on by default. Turning off OIDC registration blocks only new user creation; existing bound identities can still sign in. Email registration requires SMTP and uses a short-lived verification code, while the SMTP password is stored only in the Secret Store. Administrators can also decide whether passwordless accounts may add a local password under Account → Security. Setting or changing a password revokes the user's existing sessions and requires a fresh sign-in.

For the first visit to the complete Compose stack, open:

```text
http://localhost:8088/bootstrap
```

Production mode requires a strong random `BOOTSTRAP_TOKEN` in the API process environment. Enter the same Bootstrap Token on this page to create the first administrator. Bootstrap is unavailable when the environment value is missing and rejects mismatched values; development mode does not validate this field. After initialization, rotate or remove this one-time credential from the deployment configuration or secret manager.

The first administrator can also choose "Keep me signed in"; its session and remember-login behavior is the same as a normal local sign-in.

## Use the dashboard to choose the next action

After sign-in, the dashboard answers “what needs attention now” instead of presenting a wall of resource totals:

- The work summary shows active builds and releases, consecutive failures, and available clusters.
- Attention items group consecutive failures by application or deployment target. A later successful event closes the corresponding item.
- Recent activity combines build, release, hook, route, certificate, and other platform events with contextual links.
- Platform readiness reports clusters and registries available to the current account without treating mere existence as runtime health.

The dashboard reads this aggregation from a single `GET /api/v1/dashboard` endpoint. Future dashboard modules extend this stable response rather than making the browser compose multiple low-level list APIs.

## Create the first project space

A project space keeps the applications, members, and runtime settings for one product or team together. Think of it as that product's workspace inside the platform.

Suggested first values:

| Field | Suggestion |
| --- | --- |
| Name | Product or team name |
| Slug | Lowercase English with hyphens |
| Members | Start with yourself, invite others later |

The project space list defaults to spaces related to the current user. Platform administrators can switch the scope to all project spaces when they need global maintenance.

On mobile, management lists prioritize primary information such as the resource name. The action column only occupies the width required by its current controls, while the remaining table can still scroll horizontally when needed.

## Create the first application

An application represents one independently deployable service. For the first run, create a basic application:

- Fill in name.
- Fill in a short lowercase slug.
- Leave runtime details for later.

Service ports, image settings, Dockerfile paths, environment variables, and data volumes belong to deployment targets. The application profile only keeps the name, slug, and icon.

## Next

Continue to [Connect Cluster and Registry](/en/guide/workspace).

If you already have an image, start with an existing-image deployment. It is the shortest path to verify the platform, cluster, and route.

If you want repository-based builds, configure Git providers, registries, and build settings afterward.
