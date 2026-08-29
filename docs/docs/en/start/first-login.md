# First Sign-In

On the first startup against a fresh database, API creates the first administrator from deployment configuration. The platform has no browser initialization endpoint and includes no fixed administrator credentials.

## Prepare administrator configuration

Set at least these values before starting API:

```dotenv
INITIAL_ADMIN_EMAIL=admin@example.com
INITIAL_ADMIN_PASSWORD=replace-with-a-strong-8-to-72-byte-password
INITIAL_ADMIN_NAME=Platform Admin
INITIAL_ADMIN_LANGUAGE=en-US
```

The name may be empty and then falls back to the email. The language may be `zh-CN` or `en-US`. Supply the password through the deployment environment or a Kubernetes Secret, and never commit it to the repository.

These settings are consumed only when the database has never contained a user. If an active platform administrator already exists, API does not compare or overwrite its email, name, language, or password. If users exist but no active administrator remains, API refuses to start until an existing administrator is restored; configuration cannot silently create a second administrator.

## Sign in

After the API health check passes, open:

```text
https://your-platform.example.com/login
```

Sign in with the configured email and password. Change the password later through account settings; changing `INITIAL_ADMIN_PASSWORD` does not reset an existing account.

## Finish platform settings

After the first sign-in, open **Global Settings** and check at least:

1. Site name, public URL, and default language.
2. Registration policy and sign-in methods; internal platforms normally disable open registration.
3. Whether email, OIDC, AI Assistant, and billing should be enabled.
4. Data retention, security policy, and administrator accounts.

## Create a project space

A project space is a logical isolation boundary for the applications, members, and resources of one product or team. Start with a test space, then add members with the minimum required roles.

Next, [add base resources](./connect-resources) so users have a cluster, registry, and Git Provider OAuth ready to use.
