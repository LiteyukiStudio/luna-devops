# Initialize the Platform

After the platform starts, create the first administrator and finish the platform-level setup.

## Create the first administrator

Production deployments do not include a fixed administrator. Open:

```text
https://your-platform.example.com/bootstrap
```

Enter the `BOOTSTRAP_TOKEN` configured during deployment and create the administrator. Then **remove or rotate this one-time credential** in the deployment configuration or Secret manager.

> `/bootstrap` is for initial setup only. Later sign-ins should use local accounts or an OIDC identity provider configured by an administrator.

## Finish platform settings

Open **Global Settings** and check at least:

1. Site name, public URL, and default language.
2. Registration policy and sign-in methods; internal platforms normally disable open registration.
3. Whether email, OIDC, AI Assistant, and billing should be enabled.
4. Data retention, security policy, and administrator accounts.

## Create a project space

A project space is a logical isolation boundary for the applications, members, and resources of one product or team. Start with a test space, then add members with the minimum required roles.

Next, [add base resources](./connect-resources) so users have a cluster, registry, and Git Provider OAuth ready to use.
