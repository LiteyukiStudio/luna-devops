# First Time in the Console

After the platform starts, complete the few settings you actually need. There is no need to connect every external system at once. If you can sign in and create a project space, you are ready to prepare a runtime.

## Sign in or bootstrap

Compose starts the API in development mode by default. Open the sign-in page and follow the account hint shown there.

Local-account sign-in and first-administrator bootstrap both create a 24-hour browser session. "Keep me signed in" is off by default. Enabling it on a trusted device adds a per-user, 30-day HttpOnly remember cookie. After the session expires, choosing that recent account rotates the remember token and creates a new session. The browser keeps display metadata for at most three recent accounts, but never stores passwords, tokens, or session cookies. Signing out, disabling the account, changing its password, or changing its role revokes the related sessions and remember tokens.

If you switch to production mode, visit this page the first time:

```text
http://localhost:8088/bootstrap
```

Production mode requires a strong random `BOOTSTRAP_TOKEN` in the API process environment. Enter the same Bootstrap Token on this page to create the first administrator. Bootstrap is unavailable when the environment value is missing and rejects mismatched values; development mode does not validate this field. After initialization, rotate or remove this one-time credential from the deployment configuration or secret manager.

The first administrator can also choose "Keep me signed in"; its session and remember-login behavior is the same as a normal local sign-in.

## Create the first project space

A project space keeps the applications, members, and runtime settings for one product or team together. Think of it as that product's workspace inside the platform.

Suggested first values:

| Field | Suggestion |
| --- | --- |
| Name | Product or team name |
| Slug | Lowercase English with hyphens |
| Members | Start with yourself, invite others later |

The project space list defaults to spaces related to the current user. Platform administrators can switch the scope to all project spaces when they need global maintenance.

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
