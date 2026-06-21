# First Visit

After the platform starts, complete the smallest useful setup. The goal is not to configure every integration at once. The goal is to make one application manageable.

## Sign in or bootstrap

Compose starts the API in development mode by default. Open the sign-in page and follow the account hint shown there.

If you switch to production mode, visit this page the first time:

```text
http://localhost:8088/bootstrap
```

Use it to create the first administrator account.

## Create a project space

A project space is a workspace for teams, applications, deploy configs, and runtime resources.

The project space list defaults to spaces related to the current user. Platform administrators can switch the scope to all project spaces from the top-right control when they need global maintenance.

Suggested first values:

| Field | Suggestion |
| --- | --- |
| Name | Product or team name |
| Slug | Lowercase English with hyphens |
| Members | Start with yourself, invite others later |

## Create an application

An application represents a deployable service. For the first run, create a simple web service:

- Fill in name and slug.
- Confirm the service port.
- Leave advanced options until you need them.

## Next

If you already have an image, start with an existing-image deployment. It is the shortest path to verify the platform, cluster, and route.

If you want repository-based builds, configure Git providers, registries, and build settings afterward.
