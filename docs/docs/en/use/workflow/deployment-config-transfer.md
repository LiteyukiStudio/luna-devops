# Import and Export Deployment Configuration

A deployment can be exported as JSON and imported into another application. **Import creates a new deployment; it never overwrites an existing one or starts a build or Release.**

## Export

Open the deployment action menu and select **Export JSON**. The file contains runtime resources, build settings, release policy, and non-sensitive configuration. It excludes credentials, Secret values, current status, and history.

The file may still contain image references and ordinary configuration. Handle it as internal configuration data.

## Import

1. Select **Import JSON** on the target application's Deployments page.
2. Select the file and confirm the name, stage, and Namespace.
3. Map repositories, registries, clusters, variable sets, hooks, and project volumes listed by the preflight check.
4. Re-enter required Secrets.
5. Submit only after the preflight status says the configuration can be imported.

Public stages are `dev`, `test`, `staging`, and `prod`; `production` is normalized to `prod`. A stage must be unique in an application.

A volume mapping creates a mount relation only. It does not copy a PVC or its data. Use [Runtime Data Persistence](./runtime-data-persistence) to move data first.

After import, review the image, repository, cluster, and runtime configuration, then build or release manually.
