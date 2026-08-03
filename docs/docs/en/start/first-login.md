# First Time in the Console

After the platform starts, bootstrap an administrator and create your first project space. External services can be connected later as needed.

## Sign in or bootstrap

A full deployment does not create a fixed administrator. For the first visit, open:

```text
http://localhost:8088/bootstrap
```

Enter the `BOOTSTRAP_TOKEN` from the deployment environment and create the first administrator. Remove or rotate this one-time credential in your deployment or secret manager afterward.

You can later sign in with a local account or an OIDC provider configured by an administrator. Enable “Keep me signed in” only on a trusted device. Password, role, or sign-in method changes may require you to sign in again to protect the account.

## Dashboard

The dashboard summarizes active builds and releases, failed work that needs attention, recent activity, and cluster and registry readiness. Open an item to continue troubleshooting in its resource page.

## Create the first project space

A project space groups applications, members, and runtime configuration for one product or team. Open “Project Spaces” and create one:

| Field | Recommendation |
| --- | --- |
| Name | Use the product or team name |
| Slug | Use lowercase letters and hyphens |
| Members | Keep only yourself for the first run |

## Create the first application

An application represents one independently deployable service. Enter a name and slug; the slug cannot be changed after creation. Configure the image, port, environment variables, and volumes in a deployment target.

## Next

Continue with [Connect Cluster and Registry](/en/start/connect-resources). If an image already exists, deploy it first to validate the platform. Connect a Git provider and build from source only when needed.
